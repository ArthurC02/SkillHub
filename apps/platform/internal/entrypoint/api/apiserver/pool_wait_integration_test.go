package apiserver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
)

func TestAStarvedPoolRefusesAfterItsWaitLimitAndAHeldConnectionOutlivesIt(t *testing.T) {
	const waitLimit = 100 * time.Millisecond
	cfg, err := wiring.DatabasePoolConfig(requireDB(t).Config().ConnString(), 1, waitLimit)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	held, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			held.Release()
		}
	})
	outer, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started := time.Now()
	if _, err := pool.Acquire(outer); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquire from a pool with no free connection = %v, want it to give up with DeadlineExceeded", err)
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("waited %v for a connection, want about %v", waited, waitLimit)
	}

	time.Sleep(3 * waitLimit)
	var one int
	if err := held.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("a connection acquired before the limit failed after it: %v", err)
	}
	held.Release()
	released = true
	if again, err := pool.Acquire(context.Background()); err != nil {
		t.Fatalf("a released connection could not be acquired again: %v", err)
	} else {
		again.Release()
	}
}
