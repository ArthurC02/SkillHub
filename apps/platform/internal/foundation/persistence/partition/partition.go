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

const monthsAhead = 2

// identifierPattern gates every table name before it is formatted into DDL:
// SQL identifiers cannot be bound as parameters, so this is what keeps the
// fmt.Sprintf calls below from ever building a statement out of unsafe input.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

const boundLayout = time.RFC3339

type Report struct {
	Created []string
	Dropped []string
}

func MaintainMonthly(ctx context.Context, pool *pgxpool.Pool, table string, now time.Time, retention time.Duration) (Report, error) {
	var report Report
	if !identifierPattern.MatchString(table) {
		return report, fmt.Errorf("partition: %q is not a bare lower-case table identifier", table)
	}

	if retention <= 0 {
		return report, fmt.Errorf("partition: %s needs a positive retention window, got %s", table, retention)
	}
	now = now.UTC()

	existing, err := childPartitions(ctx, pool, table)
	if err != nil {
		return report, err
	}

	for _, name := range expiredMonths(table, existing, now, retention) {

		if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)); err != nil {
			return report, fmt.Errorf("partition: drop %s: %w", name, err)
		}
		report.Dropped = append(report.Dropped, name)
	}

	created, err := createUpcoming(ctx, pool, table, existing, now)
	report.Created = created
	return report, err
}

func CreateUpcoming(ctx context.Context, pool *pgxpool.Pool, table string, now time.Time) (Report, error) {
	var report Report
	if !identifierPattern.MatchString(table) {
		return report, fmt.Errorf("partition: %q is not a bare lower-case table identifier", table)
	}
	now = now.UTC()

	existing, err := childPartitions(ctx, pool, table)
	if err != nil {
		return report, err
	}
	created, err := createUpcoming(ctx, pool, table, existing, now)
	report.Created = created
	return report, err
}

func createUpcoming(
	ctx context.Context, pool *pgxpool.Pool, table string, existing []string, now time.Time,
) ([]string, error) {
	present := make(map[string]bool, len(existing))
	for _, name := range existing {
		present[name] = true
	}
	var created []string
	for _, start := range upcomingMonths(now) {
		name := monthName(table, start)
		if present[name] {
			continue
		}
		if err := createMonth(ctx, pool, table, name, start); err != nil {
			return created, err
		}
		created = append(created, name)
	}
	return created, nil
}

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

func createMonth(ctx context.Context, pool *pgxpool.Pool, table, name string, start time.Time) error {
	end := start.AddDate(0, 1, 0)
	statement := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		name, table, start.Format(boundLayout), end.Format(boundLayout))
	_, err := pool.Exec(ctx, statement)
	if err == nil {
		return nil
	}

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

func upcomingMonths(now time.Time) []time.Time {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := make([]time.Time, 0, monthsAhead+1)
	for i := 0; i <= monthsAhead; i++ {
		months = append(months, start.AddDate(0, i, 0))
	}
	return months
}

func monthName(table string, start time.Time) string {
	return fmt.Sprintf("%s_%04d_%02d", table, start.Year(), int(start.Month()))
}

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
		// Compared against the month's exclusive end, not its start, so a month
		// is only expired once every row it could hold is past the cutoff.
		end := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC)
		if !end.After(cutoff) {
			expired = append(expired, name)
		}
	}
	sort.Strings(expired)
	return expired
}
