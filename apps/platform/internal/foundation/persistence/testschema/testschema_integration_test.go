package testschema

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset")
		}
		t.Skip("SKILLHUB_TEST_DATABASE_URL is unset")
	}
	return dsn
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func prepared(t *testing.T, dsn string) string {
	t.Helper()
	clone, drop, err := Prepare(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(drop)
	return clone
}

func TestASecondHolderWaitsUntilTheFirstUnlocks(t *testing.T) {
	dsn := requireDSN(t)
	first, second := connect(t, dsn), connect(t, dsn)
	unlock, err := lock(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}

	if err := lockWithin(second, 300*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		unlock()
		t.Fatalf("second lock while the first holds it: err = %v, want it to wait until the deadline", err)
	}

	unlock()
	retried := connect(t, dsn)
	unlockSecond, err := lock(context.Background(), retried)
	if err != nil {
		t.Fatalf("second lock after the first unlocked: %v", err)
	}
	unlockSecond()
}

func lockWithin(conn *pgx.Conn, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	unlock, err := lock(ctx, conn)
	if err == nil {
		unlock()
	}
	return err
}

func TestPrepareGivesEachCallerItsOwnMigratedDatabase(t *testing.T) {
	dsn := requireDSN(t)
	ctx := context.Background()
	one, two := prepared(t, dsn), prepared(t, dsn)
	if one == dsn || one == two {
		t.Fatalf("Prepare returned %q then %q from %q, want two databases of their own", one, two, dsn)
	}

	first := connect(t, one)
	var migrated int
	if err := first.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN ('users', 'workspaces', 'skills')`).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if migrated != 3 {
		t.Fatalf("migrated tables in the clone = %d, want users, workspaces and skills", migrated)
	}

	email := "clone-" + strings.TrimPrefix(one, dsn) + "@example.test"
	if _, err := first.Exec(ctx, "INSERT INTO users (email, display_name) VALUES ($1, $1)", email); err != nil {
		t.Fatal(err)
	}
	var leaked int
	if err := connect(t, two).QueryRow(ctx, "SELECT count(*) FROM users WHERE email = $1", email).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("a row written in one clone is visible in the other (%d rows)", leaked)
	}
}

func TestDropRemovesTheCloneOnceItsConnectionsAreClosed(t *testing.T) {
	dsn := requireDSN(t)
	ctx := context.Background()
	clone, drop, err := Prepare(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, clonePrefix) {
		t.Fatalf("clone database is named %q, want the %q prefix", name, clonePrefix)
	}
	_ = conn.Close(ctx)

	drop()

	var exists bool
	if err := connect(t, dsn).QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("database %s still exists after drop", name)
	}
}

func TestPrepareRefusesADatabaseThatDoesNotExist(t *testing.T) {
	dsn := requireDSN(t)
	_, _, err := Prepare(context.Background(), withDatabase(dsn, "skillhub_never_created_test"))
	if err == nil {
		t.Fatal("Prepare against a missing database returned no error")
	}
}

func TestOnlyALocalDatabaseNamedForTestsIsAcceptedBeforeAnythingIsCreated(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
		"postgres://user:pass@localhost/postgres",
	} {
		if err := ValidateDestructiveURL(raw); err == nil {
			t.Errorf("unsafe URL accepted: %s", raw)
		}
		if _, _, err := Prepare(context.Background(), raw); err == nil {
			t.Errorf("Prepare accepted the unsafe URL %s", raw)
		}
	}
	if err := ValidateDestructiveURL("postgres://user:pass@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test URL rejected: %v", err)
	}
}
