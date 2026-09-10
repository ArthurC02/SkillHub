package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSetAccessRestrictionRejectsABlankReason(t *testing.T) {
	for name, reason := range map[string]string{
		"empty":      "",
		"whitespace": "  ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := SetAccessRestriction(context.Background(), nil, pgtype.UUID{}, &reason); !errors.Is(err, ErrEmptyRestriction) {
				t.Errorf("SetAccessRestriction err = %v", err)
			}
		})
	}
}

func TestSetAccessRestrictionSerializesConcurrentOperators(t *testing.T) {
	pool := requireRegistryDB(t)
	_, skillID := seedSkill(t, pool, "restriction-race")
	ctx := context.Background()

	conns := make([]*pgxpool.Conn, 2)
	for i := range conns {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		conns[i] = conn
	}
	tx1, err := conns[0].Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx) //nolint:errcheck // no-op after commit
	tx2, err := conns[1].Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck // no-op after commit

	first := "license-review"
	firstBefore, err := SetAccessRestriction(ctx, tx1, skillID, &first)
	if err != nil {
		t.Fatal(err)
	}
	if firstBefore.AccessRestriction != nil {
		t.Fatalf("before-state of the first change = %v, want nil on a fresh skill", *firstBefore.AccessRestriction)
	}

	if !firstBefore.WorkspaceID.Valid {
		t.Error("before-state carries no workspace id")
	}

	type outcome struct {
		before RestrictionBefore
		err    error
	}
	out := make(chan outcome, 1)
	go func() {
		second := "takedown-review"
		before, err := SetAccessRestriction(ctx, tx2, skillID, &second)
		out <- outcome{before, err}
	}()

	if !waitsOnLock(t, pool, conns[1].Conn().PgConn().PID()) {
		t.Fatal("the second operator did not block on the first transaction; the row was never locked")
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	second := <-out
	if second.err != nil {
		t.Fatal(second.err)
	}

	if second.before.AccessRestriction == nil || *second.before.AccessRestriction != first {
		t.Errorf("second operator's before-state = %v, want %q - it recorded a hold that was already lifted or never seen",
			second.before.AccessRestriction, first)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var final string
	if err := pool.QueryRow(ctx, `SELECT access_restriction FROM skills WHERE id = $1`, skillID).Scan(&final); err != nil {
		t.Fatal(err)
	}
	if final != "takedown-review" {
		t.Errorf("stored restriction = %q, want the second operator's", final)
	}
}

func waitsOnLock(t *testing.T, pool *pgxpool.Pool, pid uint32) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := pool.QueryRow(context.Background(),
			`SELECT COALESCE((SELECT wait_event_type = 'Lock' FROM pg_stat_activity WHERE pid = $1), false)`,
			pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
