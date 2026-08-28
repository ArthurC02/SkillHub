package partition

// Integration test against a real PostgreSQL. Set SKILLHUB_TEST_DATABASE_URL and
// the tests below run; leave it unset and they skip, so CI without a database
// reports "skipped" rather than a false pass.
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.
//
// It resets rather than working with whatever is there because the subject is a
// set of partitions: a leftover month from another package's fixtures would make
// "which partitions exist" a different question on every run. The reset runs
// under the same session advisory lock apiserver, eval and registry take, since
// `go test ./...` runs packages concurrently and one package's DROP SCHEMA
// landing mid-test in another is exactly what that lock exists to prevent.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const partitionDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

// The two partitioned tables the owning contexts declare
// (trace.PartitionedTable, analytics.PartitionedTable). Spelled out rather than
// imported: this is the generic package, and importing a bounded context here —
// even from a test — would be a cycle, since both owners import this one.
// cmd/maintenance's TestEveryPartitionedTableIsRotated is what keeps these two
// names in step with the migrations and with the owners' constants.
const (
	analyticsTable = "analytics_events"
	traceTable     = "trace_events"
)

var partitionPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(partitionDBURLEnv)
	if dsn == "" {
		// 02:PORT-004. Without this, an unset or misspelled URL is indistinguishable
		// from a passing run: every database test removes itself and go test still
		// prints ok. CI sets SKILLHUB_REQUIRE_DB=1 so the service failing to come up
		// is a red build rather than a quiet one.
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", partitionDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run()) // the database tests skip; see requirePartitionDB
	}
	if err := validateDestructivePartitionDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migratePartitionSchema(ctx, pool); err != nil {
		panic(err)
	}
	partitionPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

// The whole job in one sequence, because the interesting properties are about
// what one run leaves behind for the next: months created ahead, months dropped
// once every row they can hold is out of window, and the rows themselves gone
// with them.
//
// analytics_events rather than trace_events for the row half: both have the same
// partition shape, but trace_events' rows need a workspace, a skill, a version
// and a run to hang off, and a fixture chain that long is four more ways for
// this test to fail for reasons that are not partitioning. trace_events' own
// rotation is covered below, where no rows are needed.
func TestMonthlyPartitionsRollForwardAndExpireWithTheirRows(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()
	const forever = 3650 * 24 * time.Hour

	// The migrations ship August 2026 and the default. A run dated 2026-09-01
	// adds September through November.
	report, err := MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.September, 1), forever)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "created", report.Created,
		"analytics_events_2026_09", "analytics_events_2026_10", "analytics_events_2026_11")
	assertNames(t, "dropped", report.Dropped)

	// One row in a month that will expire, one in a month that will not. Both
	// land in a real monthly partition, which the assertions below confirm by
	// dropping one of the partitions and looking for the row through the parent.
	insertAnalyticsEvent(t, pool, "expired-row", date(2026, time.August, 10))
	insertAnalyticsEvent(t, pool, "in-window-row", date(2026, time.October, 20))

	// 2026-11-15 with a 30 day window: the cutoff is 2026-10-16, so August and
	// September are wholly out of window and October is not.
	report, err = MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.November, 15), 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "dropped", report.Dropped,
		"analytics_events_2026_08", "analytics_events_2026_09")
	assertNames(t, "created", report.Created,
		"analytics_events_2026_12", "analytics_events_2027_01")

	if countAnalyticsEvents(t, pool, "expired-row") != 0 {
		t.Error("a row in an expired month survived the partition drop")
	}
	if countAnalyticsEvents(t, pool, "in-window-row") != 1 {
		t.Error("a row inside the retention window was removed")
	}

	// The default partition is never a candidate. Without it, the first write
	// into a month nobody created would fail and the event would be lost.
	if !contains(childPartitionNames(t, pool, analyticsTable), "analytics_events_default") {
		t.Fatal("the default partition was dropped")
	}

	// Re-running the identical call is a no-op, which is what makes this safe to
	// put on a cron that may fire twice.
	report, err = MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.November, 15), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	assertNames(t, "created", report.Created)
	assertNames(t, "dropped", report.Dropped)
}

// The failure direction. A default partition holding rows in the month being
// attached makes the CREATE impossible, and that has to surface: it is the only
// signal an operator gets that db/migrations/0019's one-off drain has come due.
func TestAttachingAMonthTheDefaultAlreadyHoldsFailsWithTheDrainInstructions(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()
	const forever = 3650 * 24 * time.Hour

	// No partition covers May 2027, so this row goes to the default.
	insertAnalyticsEvent(t, pool, "stranded-in-default", date(2027, time.May, 5))

	report, err := MaintainMonthly(ctx, pool, analyticsTable, date(2027, time.May, 10), forever)
	if err == nil {
		t.Fatal("attaching a month over occupied default rows reported success")
	}
	for _, want := range []string{"analytics_events_2027_05", "analytics_events_default", "DETACH PARTITION"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
	assertNames(t, "created", report.Created)

	// The row is still there: a refused CREATE must not have moved or lost it.
	if countAnalyticsEvents(t, pool, "stranded-in-default") != 1 {
		t.Error("the row in the default partition did not survive the failed attach")
	}
}

// The other owner's table, rotated by the same mechanism. No rows, so no fixture
// chain: what this proves is that the create path works against trace_events as
// it is actually declared in db/migrations/0004 and 0019, default partition and
// all.
func TestTraceEventsRollsForwardToo(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()

	report, err := MaintainMonthly(ctx, pool, traceTable, date(2026, time.September, 1), 3650*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "created", report.Created,
		"trace_events_2026_09", "trace_events_2026_10", "trace_events_2026_11")
	if !contains(childPartitionNames(t, pool, traceTable), "trace_events_default") {
		t.Fatal("the default partition was dropped")
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func requirePartitionDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if partitionPool == nil {
		t.Skipf("%s not set; skipping partition rotation test", partitionDBURLEnv)
	}
	return partitionPool
}

// insertAnalyticsEvent writes one funnel row at a chosen instant. Raw SQL rather
// than internal/analytics' constructors: this package is generic and must not
// import a bounded context, and what is under test is the partitioning, not the
// event vocabulary.
func insertAnalyticsEvent(t *testing.T, pool *pgxpool.Pool, session string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO analytics_events (event_name, session_id, occurred_at) VALUES ('session_started', $1, $2)`,
		session, at); err != nil {
		t.Fatal(err)
	}
}

// countAnalyticsEvents reads through the parent table, so a row that went away
// with its partition is indistinguishable from one that was deleted — which is
// the point: retention has to be observable from where the data is read.
func countAnalyticsEvents(t *testing.T, pool *pgxpool.Pool, session string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM analytics_events WHERE session_id = $1`, session).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func childPartitionNames(t *testing.T, pool *pgxpool.Pool, table string) []string {
	t.Helper()
	names, err := childPartitions(context.Background(), pool, table)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func assertNames(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// validateDestructivePartitionDatabaseURL refuses to point the schema drop below
// at anything that is not an obviously disposable local database.
func validateDestructivePartitionDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", partitionDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", partitionDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", partitionDBURLEnv)
	}
	return nil
}

func TestDestructivePartitionDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
	} {
		if err := validateDestructivePartitionDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructivePartitionDatabaseURL("postgres://u:p@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func migratePartitionSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		// No arguments means the simple protocol, so a file with several
		// statements applies as one batch.
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// lockTestSchema serialises the packages that reset this database — the same
// session advisory lock apiserver, eval and registry take, held on one
// connection for the whole package run. A package that resets without it sees,
// or causes, "relation does not exist" halfway through somebody else's tests.
func lockTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	return func() {
		_, _ = conn.Exec(ctx,
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	}
}
