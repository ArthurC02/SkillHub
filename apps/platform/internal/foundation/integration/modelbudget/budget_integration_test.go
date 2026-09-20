package modelbudget

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const budgetDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var budgetPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(budgetDBURLEnv)
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have "+
				"skipped every database test and still reported success\n", budgetDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	if err := validateDestructiveBudgetDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migrateBudgetSchema(ctx, pool); err != nil {
		panic(err)
	}
	budgetPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

func validateDestructiveBudgetDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", budgetDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", budgetDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", budgetDBURLEnv)
	}
	return nil
}

func TestDestructiveBudgetDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
	} {
		if err := validateDestructiveBudgetDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructiveBudgetDatabaseURL("postgres://u:p@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func migrateBudgetSchema(ctx context.Context, pool *pgxpool.Pool) error {
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

// requireBudgetDB gives each test its own endpoint kind. Audit events are
// immutable, so tests cannot clear them and must not see each other's.
func requireBudgetDB(t *testing.T) (*Service, Endpoint) {
	t.Helper()
	if budgetPool == nil {
		t.Skipf("%s not set; skipping model budget database test", budgetDBURLEnv)
	}
	e := Endpoint{Kind: t.Name(), Deadline: judge.Deadline}
	if _, err := budgetPool.Exec(context.Background(),
		"DELETE FROM model_call_budgets WHERE kind = $1", e.Kind); err != nil {
		t.Fatalf("clearing budgets: %v", err)
	}
	return &Service{Pool: budgetPool, Endpoints: []Endpoint{e}}, e
}

func auditRows(t *testing.T, kind string) []map[string]any {
	t.Helper()
	rows, err := budgetPool.Query(context.Background(),
		"SELECT action, resource_type, metadata FROM audit_events "+
			"WHERE metadata->>'kind' = $1 ORDER BY id", kind)
	if err != nil {
		t.Fatalf("reading audit events: %v", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var action, resource string
		var metadata map[string]any
		if err := rows.Scan(&action, &resource, &metadata); err != nil {
			t.Fatalf("scanning audit event: %v", err)
		}
		metadata["action"], metadata["resource_type"] = action, resource
		out = append(out, metadata)
	}
	return out
}

func TestTheOperatorsCeilingIsWhatTheNextCallIsGiven(t *testing.T) {
	svc, judge := requireBudgetDB(t)
	ctx := context.Background()
	compiled := time.Duration(judge.Ceiling()) * time.Second

	if got := svc.Within(ctx, judge); got != compiled {
		t.Fatalf("with nothing set, Within = %s, want the compiled %s", got, compiled)
	}

	if _, err := svc.Set(ctx, judge.Kind, 20, "the gateway is slow today", pgtype.UUID{}); err != nil {
		t.Fatalf("setting a ceiling: %v", err)
	}
	if got := svc.Within(ctx, judge); got != 20*time.Second {
		t.Errorf("Within = %s after the operator set 20s, want 20s; the setting page would report a "+
			"number the calls never use", got)
	}

	if err := svc.Clear(ctx, judge.Kind, "the gateway recovered", pgtype.UUID{}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if got := svc.Within(ctx, judge); got != compiled {
		t.Errorf("Within = %s after clearing, want the compiled %s", got, compiled)
	}
}

func TestEveryChangeLeavesOneAuditEventNamingTheReasonAndBothValues(t *testing.T) {
	svc, judge := requireBudgetDB(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, judge.Kind, 20, "the gateway is slow today", pgtype.UUID{}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	if _, err := svc.Set(ctx, judge.Kind, 30, "still slow, loosening further", pgtype.UUID{}); err != nil {
		t.Fatalf("resetting: %v", err)
	}
	if err := svc.Clear(ctx, judge.Kind, "the gateway recovered", pgtype.UUID{}); err != nil {
		t.Fatalf("clearing: %v", err)
	}

	events := auditRows(t, judge.Kind)
	if len(events) != 3 {
		t.Fatalf("got %d audit events, want one per change", len(events))
	}
	for i, want := range []struct {
		reason, before, after any
	}{
		{"the gateway is slow today", "default", float64(20)},
		{"still slow, loosening further", float64(20), float64(30)},
		{"the gateway recovered", float64(30), "default"},
	} {
		got := events[i]
		if got["action"] != "model_budget.set" || got["resource_type"] != "model_budget" {
			t.Errorf("event %d is %v/%v, want the model budget action", i, got["action"], got["resource_type"])
		}
		if got["kind"] != judge.Kind || got["reason"] != want.reason {
			t.Errorf("event %d names %v/%v, want %v/%v", i, got["kind"], got["reason"], judge.Kind, want.reason)
		}
		if got["before"] != want.before || got["after"] != want.after {
			t.Errorf("event %d went %v -> %v, want %v -> %v; without both values the record does not "+
				"say what changed", i, got["before"], got["after"], want.before, want.after)
		}
	}
}

func TestARefusedChangeLeavesNeitherASettingNorAnEvent(t *testing.T) {
	svc, judge := requireBudgetDB(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, judge.Kind, judge.Ceiling()+1, "past the ceiling",
		pgtype.UUID{}); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("Set past the ceiling = %v, want %v", err, ErrOutOfRange)
	}
	if _, err := svc.Get(ctx, judge.Kind); !errors.Is(err, ErrNotSet) {
		t.Errorf("a refused change still stored a value: %v", err)
	}
	if events := auditRows(t, judge.Kind); len(events) != 0 {
		t.Errorf("a refused change wrote %d audit events; a refusal is not a change", len(events))
	}
}

func TestAStoredValueTheCompiledDeadlineNoLongerAllowsIsIgnored(t *testing.T) {
	svc, judge := requireBudgetDB(t)
	ctx := context.Background()
	if _, err := svc.Set(ctx, judge.Kind, judge.Ceiling(), "at the ceiling", pgtype.UUID{}); err != nil {
		t.Fatalf("setting at the ceiling: %v", err)
	}

	shortened := Endpoint{Kind: judge.Kind, Deadline: Margin + 10*time.Second}
	want := time.Duration(shortened.Ceiling()) * time.Second
	if got := svc.Within(ctx, shortened); got != want {
		t.Errorf("Within = %s for a deadline that was shortened under a stored value, want %s; a "+
			"value left over from a longer deadline would outlive the caller", got, want)
	}
}
