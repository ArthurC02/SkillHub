package partition

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

const (
	analyticsTable = "analytics_events"
	traceTable     = "trace_events"
)

var partitionPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(partitionDBURLEnv)
	if dsn == "" {

		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", partitionDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
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

func TestMonthlyPartitionsRollForwardAndExpireWithTheirRows(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()
	const forever = 3650 * 24 * time.Hour

	report, err := MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.September, 1), forever)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "created", report.Created,
		"analytics_events_2026_09", "analytics_events_2026_10", "analytics_events_2026_11")
	assertNames(t, "dropped", report.Dropped)

	insertAnalyticsEvent(t, pool, "expired-row", date(2026, time.August, 10))
	insertAnalyticsEvent(t, pool, "in-window-row", date(2026, time.October, 20))

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

	if !contains(childPartitionNames(t, pool, analyticsTable), "analytics_events_default") {
		t.Fatal("the default partition was dropped")
	}

	report, err = MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.November, 15), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	assertNames(t, "created", report.Created)
	assertNames(t, "dropped", report.Dropped)
}

func TestAttachingAMonthTheDefaultAlreadyHoldsFailsWithTheDrainInstructions(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()
	const forever = 3650 * 24 * time.Hour

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

	if countAnalyticsEvents(t, pool, "stranded-in-default") != 1 {
		t.Error("the row in the default partition did not survive the failed attach")
	}
}

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

func requirePartitionDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if partitionPool == nil {
		t.Skipf("%s not set; skipping partition rotation test", partitionDBURLEnv)
	}
	return partitionPool
}

func insertAnalyticsEvent(t *testing.T, pool *pgxpool.Pool, session string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO analytics_events (event_name, session_id, occurred_at) VALUES ('session_started', $1, $2)`,
		session, at); err != nil {
		t.Fatal(err)
	}
}

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

		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

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
