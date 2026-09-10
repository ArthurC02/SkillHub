package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

func TestExpiredAuditEventsAreSweptAndRecentOnesAreNot(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()

	const action = "test.audit_retention_sweep"
	_, err := pool.Exec(ctx, `
		INSERT INTO audit_events (action, resource_type, created_at) VALUES
			($1, 'test', now() - interval '500 days'),
			($1, 'test', now() - interval '399 days'),
			($1, 'test', now() - interval '1 day')`, action)
	if err != nil {
		t.Fatalf("seed audit events: %v", err)
	}
	t.Cleanup(func() {

		tx, err := pool.Begin(ctx)
		if err != nil {
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
			return
		}
		_, _ = tx.Exec(ctx, `DELETE FROM audit_events WHERE action = $1`, action)
		_ = tx.Commit(ctx)
	})

	if _, err := audit.PurgeExpired(ctx, pool, 400*24*time.Hour); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE action = $1`, action).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}

	if remaining != 2 {
		t.Fatalf("expected the 500-day event gone and the other two kept, got %d rows", remaining)
	}
	var oldest time.Time
	if err := pool.QueryRow(ctx,
		`SELECT min(created_at) FROM audit_events WHERE action = $1`, action).Scan(&oldest); err != nil {
		t.Fatalf("min: %v", err)
	}
	if time.Since(oldest) > 400*24*time.Hour {
		t.Fatalf("an event older than the window survived the sweep: %s", oldest)
	}
}

func TestAnAuditEventCannotBeDeletedWithoutTheNamedExemption(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()

	const action = "test.audit_delete_refused"
	if _, err := pool.Exec(ctx,
		`INSERT INTO audit_events (action, resource_type) VALUES ($1, 'test')`, action); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
			return
		}
		_, _ = tx.Exec(ctx, `DELETE FROM audit_events WHERE action = $1`, action)
		_ = tx.Commit(ctx)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM audit_events WHERE action = $1`, action); err == nil {
		t.Fatal("a plain DELETE on audit_events succeeded; the immutability trigger is not in force")
	}
}

func TestTheAuditSweepRefusesToRunWithoutAWindow(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()

	for _, retention := range []time.Duration{0, -time.Hour} {
		n, err := audit.PurgeExpired(ctx, pool, retention)
		if err == nil {
			t.Fatalf("retention %s was accepted; it must be refused", retention)
		}
		if n != 0 {
			t.Fatalf("retention %s reported %d rows removed", retention, n)
		}
	}
}
