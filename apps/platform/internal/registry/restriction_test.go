package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// A nil transaction is deliberate: every case here must be rejected before the
// function reaches the database, and a nil pointer dereference is a louder
// failure than a passing assertion would be. That is also the assertion that the
// blank check runs before the row is locked - a lock needs a transaction.
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

// TestSetAccessRestrictionSerializesConcurrentOperators is the reason DDD-031
// moved the FOR UPDATE in here from internal/catalog (ADR-035 B 組).
//
// Two operators change the same hold at once. The before-state this function
// returns is what the audit event records, so the second one must see the first
// one's committed value and not the value that was there when it started. Delete
// the LockSkillForRestriction call - or move it back out to a caller that this
// function cannot see - and the second operator reads its own MVCC snapshot,
// records "there was no hold", and writes an audit event that is false.
//
// It has to be a real database: the failure is Read Committed visibility, which
// no fake can reproduce and no unit test can express.
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
	// catalog's audit event scopes itself with this, so an empty one would make
	// a cross-workspace operator action unreviewable in the workspace it hit.
	if !firstBefore.WorkspaceID.Valid {
		t.Error("before-state carries no workspace id")
	}

	type outcome struct {
		before gen.LockSkillForRestrictionRow
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
	// The assertion the lock exists for. Without FOR UPDATE this is nil: the
	// second transaction's snapshot predates the first one's commit.
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

// waitsOnLock reports whether pid is parked on a lock, which is how a test
// observes "the other transaction is being made to wait" without a sleep that
// would be a guess either way. Same probe as apiserver's concurrency tests.
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
