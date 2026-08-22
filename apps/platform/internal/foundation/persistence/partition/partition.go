// Package partition rotates monthly range partitions: it pre-creates the months
// that are about to be written to, and drops the ones whose data has aged past a
// retention window.
//
// Generic (ADR-032 §1 Generic row): the mechanism is identical for every table
// declared PARTITION BY RANGE on a timestamptz, so nothing here knows what any
// of them mean. The table name and the retention window are supplied by the
// context that owns them — trace for `trace_events`, analytics for
// `analytics_events` — and neither owner can reach the other's table through
// this package, because this package only ever touches the one name it is
// handed.
//
// # Why the DDL is assembled from formatted strings
//
// A partition name is an identifier and identifiers cannot be parameterised, so
// the statements below are necessarily built with fmt.Sprintf. Every identifier
// and every bound literal that reaches a statement comes from exactly two
// sources: the caller's table name, which must match [identifierPattern] before
// anything is executed, and a month boundary this package computed from the
// `now` it was given. Nothing read out of the database, and nothing that
// originated with a user or an HTTP request, is ever formatted into a statement
// here — the introspection query passes the table as a bind parameter, and its
// results are only ever compared against names this package derived itself.
//
// These catalog reads and DDL are intentionally raw because sqlc cannot safely
// bind PostgreSQL identifiers. `devctl automation-check` sees SELECT/CREATE/DROP
// and permits only the three exact functions named in raw_sql_allow; another
// function in this file does not inherit their exemption.
package partition

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// monthsAhead is how far past the current month partitions are pre-created.
// Two, not one, is operational slack rather than a policy: this job is invoked
// by whatever cron the deployment already runs (iron rule 6 keeps the "when"
// outside the code), and a monthly schedule that misses two consecutive runs
// would otherwise start writing into the default partition — which is exactly
// the state 0019 describes and which no partition drop can clean up.
const monthsAhead = 2

// identifierPattern is what a table name has to look like before it is allowed
// into a statement. Deliberately narrower than Postgres allows (no quoting, no
// schema qualification, no upper case): every caller is a compile-time constant
// in the owning context, so anything else is a bug worth failing on rather than
// a case worth supporting.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// boundLayout formats the FROM/TO literals. RFC 3339 in UTC, so the bound a
// partition is created with is textually the same shape the migrations wrote by
// hand (db/migrations/0004, 0029).
const boundLayout = time.RFC3339

// Report is what one run did. Both slices are the partition names, in the order
// the statements ran, so an operator reading the log sees the actual effect
// rather than "ok".
type Report struct {
	Created []string
	Dropped []string
}

// MaintainMonthly brings one partitioned table's set of monthly partitions into
// step with `now`: partitions whose whole month ended before now-retention are
// dropped, and the current month plus [monthsAhead] are created.
//
// Idempotent and safe to re-run: a partition that already exists is skipped
// (and CREATE ... IF NOT EXISTS covers the race with a concurrent run), and a
// partition that is already gone is a no-op DROP ... IF EXISTS. Running it twice
// in a row produces an empty second Report.
//
// Retention runs before creation on purpose. A creation that collides with the
// default partition is a hard stop (see createMonth), and if that ran first, one
// stuck month would silently suspend retention on every other one — a disk
// filling up while the job reports a failure that looks unrelated.
//
// The Report is returned even when the error is non-nil: work already done is
// what tells an operator how far the run got.
func MaintainMonthly(ctx context.Context, pool *pgxpool.Pool, table string, now time.Time, retention time.Duration) (Report, error) {
	var report Report
	if !identifierPattern.MatchString(table) {
		return report, fmt.Errorf("partition: %q is not a bare lower-case table identifier", table)
	}
	// Fail-closed, mirroring the call sites: a zero or negative window would
	// make "expired" mean "everything up to now", and this job's mistakes are
	// not recoverable.
	if retention <= 0 {
		return report, fmt.Errorf("partition: %s needs a positive retention window, got %s", table, retention)
	}
	now = now.UTC()

	existing, err := childPartitions(ctx, pool, table)
	if err != nil {
		return report, err
	}

	for _, name := range expiredMonths(table, existing, now, retention) {
		// Bounded by construction: only names this package could have created
		// itself are candidates, so the list is the set of months that exist.
		// Plain DROP TABLE rather than DETACH CONCURRENTLY + DROP — it takes a
		// brief ACCESS EXCLUSIVE lock on the parent, which is acceptable for an
		// operator-invoked job and honest about what it does.
		if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)); err != nil {
			return report, fmt.Errorf("partition: drop %s: %w", name, err)
		}
		report.Dropped = append(report.Dropped, name)
	}

	present := make(map[string]bool, len(existing))
	for _, name := range existing {
		present[name] = true
	}
	for _, start := range upcomingMonths(now) {
		name := monthName(table, start)
		if present[name] {
			continue
		}
		if err := createMonth(ctx, pool, table, name, start); err != nil {
			return report, err
		}
		report.Created = append(report.Created, name)
	}
	return report, nil
}

// childPartitions names every partition currently attached to table, including
// the default one. The table travels as a bind parameter; the names come back as
// data and are never formatted into a statement — expiredMonths only uses them
// to decide whether a name this package can derive is already there.
func childPartitions(ctx context.Context, pool *pgxpool.Pool, table string) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT c.relname
		FROM pg_catalog.pg_inherits i
		JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = to_regclass($1::text)`, table)
	if err != nil {
		return nil, fmt.Errorf("partition: list partitions of %s: %w", table, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("partition: list partitions of %s: %w", table, err)
	}
	sort.Strings(names)
	return names, nil
}

// createMonth attaches one month.
func createMonth(ctx context.Context, pool *pgxpool.Pool, table, name string, start time.Time) error {
	end := start.AddDate(0, 1, 0)
	statement := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		name, table, start.Format(boundLayout), end.Format(boundLayout))
	_, err := pool.Exec(ctx, statement)
	if err == nil {
		return nil
	}
	// 23514 check_violation is what Postgres raises when the default partition
	// already holds rows inside the new bound: attaching the month would move
	// them, so the whole statement is refused. This is not a failure to paper
	// over. It is the only signal an operator gets that the drain 0019 predicted
	// has become necessary, so the message says what to do rather than what
	// went wrong.
	//
	// ponytail: the drain is not automated. Ceiling — a deployment that reaches
	// this state stays stuck until a human runs the four statements below, and
	// on a large default partition they need a maintenance window because the
	// detach and the re-attach both take ACCESS EXCLUSIVE. Upgrade path: do it
	// here in one transaction, driven off a row count that says how long it will
	// take. Not built now because the job that exists from today onwards is what
	// stops the default from filling up in the first place; the drain is a
	// one-off for the months that already landed there.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		from, to := start.Format(boundLayout), end.Format(boundLayout)
		return fmt.Errorf(
			"partition: cannot attach %s because %s_default already holds rows in [%s, %s): "+
				"drain the default first, in one transaction — "+
				"ALTER TABLE %s DETACH PARTITION %s_default; "+
				"CREATE TABLE %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s'); "+
				"move the rows in that range out of %s_default into %s; "+
				"ALTER TABLE %s ATTACH PARTITION %s_default DEFAULT; "+
				"then re-run this job (db/migrations/0019 named this drain when it added the default): %w",
			name, table, from, to,
			table, table,
			name, table, from, to,
			table, table,
			table, table,
			err)
	}
	return fmt.Errorf("partition: create %s: %w", name, err)
}

// upcomingMonths is the set of months that must exist after this run: the one
// `now` falls in, and [monthsAhead] after it.
func upcomingMonths(now time.Time) []time.Time {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := make([]time.Time, 0, monthsAhead+1)
	for i := 0; i <= monthsAhead; i++ {
		months = append(months, start.AddDate(0, i, 0))
	}
	return months
}

// monthName is the one naming rule in this package, and it is also what makes
// dropping safe: it is the shape the migrations already used
// (trace_events_2026_08, analytics_events_2026_08), and expiredMonths will only
// ever consider a name that matches it.
func monthName(table string, start time.Time) string {
	return fmt.Sprintf("%s_%04d_%02d", table, start.Year(), int(start.Month()))
}

// expiredMonths picks the partitions whose month ended at or before
// now-retention. A partition is only a candidate if its name is one monthName
// could have produced, which has two consequences worth stating: the DEFAULT
// partition is never dropped (`<table>_default` cannot parse as a month), and
// neither is any partition somebody attached by hand under another name. This
// job removes only what it recognises as its own.
//
// The comparison is on the month's exclusive upper bound, not its start: a
// partition is expired only once every row it can hold is older than the window,
// so an in-window month is never dropped for containing older rows too.
func expiredMonths(table string, existing []string, now time.Time, retention time.Duration) []string {
	cutoff := now.Add(-retention)
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(table) + `_(\d{4})_(\d{2})$`)
	var expired []string
	for _, name := range existing {
		match := pattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		if month < 1 || month > 12 {
			continue
		}
		end := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC)
		if !end.After(cutoff) {
			expired = append(expired, name)
		}
	}
	sort.Strings(expired)
	return expired
}
