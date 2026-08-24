package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

// The 400 day sweep PDM-006 §6 promises and gate-test/consent-and-data-policy.md
// §3 tells a participant about.
//
// Until 2026-08-25 three quarters of this mechanism existed and the fourth did
// not: 0013 names the retention in a column comment, names the index after the
// sweep, and opens the DELETE branch of enforce_immutable for it — and no query,
// no command and no caller ever deleted a row. The document a participant signs
// said 400 days; what ran said never.
//
// What makes this test able to fail rather than merely pass: the second case
// proves the trigger is live on this table, so removing the
// `SET LOCAL skillhub.purge = 'on'` line from PurgeExpired turns the first case
// red with restrict_violation rather than quietly deleting nothing.
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
		// The trigger refuses a plain DELETE, which is the whole point of the
		// table; the cleanup has to use the same named exemption the sweep does.
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
	// 500 days goes; 399 days and 1 day stay. The 399 is there on purpose: a
	// sweep that used the wrong comparison, or a window off by a unit, takes it
	// too, and a fixture with only 500 and 1 could not tell the difference.
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

// The trigger is what makes the sweep's exemption meaningful, so it is worth one
// assertion of its own: if this ever stops holding, the test above stops proving
// anything and would not say so.
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

// Fail closed. A window of zero would delete the whole trail, and "the sweep ran
// before anyone configured it" must not be how NFR-001's evidence disappears.
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
