package testschema

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func requirePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset")
		}
		t.Skip("SKILLHUB_TEST_DATABASE_URL is unset")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestASecondHolderWaitsUntilTheFirstUnlocks(t *testing.T) {
	first, second := requirePool(t), requirePool(t)
	unlock, err := Lock(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}

	if err := lockWithin(second, 300*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		unlock()
		t.Fatalf("second Lock while the first holds it: err = %v, want it to wait until the deadline", err)
	}

	unlock()
	if err := lockWithin(second, 5*time.Second); err != nil {
		t.Fatalf("second Lock after the first unlocked: %v", err)
	}
}

func lockWithin(pool *pgxpool.Pool, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	unlock, err := Lock(ctx, pool)
	if err == nil {
		unlock()
	}
	return err
}
