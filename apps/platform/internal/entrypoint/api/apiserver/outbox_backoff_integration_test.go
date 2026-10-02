package apiserver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
)

var errConsumerDown = errors.New("consumer is down")

func insertPendingOutboxEvent(t *testing.T, pool *pgxpool.Pool, workspace, aggregate pgtype.UUID, occurredAt string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO outbox_events (
			event_type, event_version, occurred_at, correlation_id, workspace_id,
			aggregate_type, aggregate_id, payload
		) VALUES ('run.cleanup_cleaned', 1, $1::timestamptz, $3::uuid, $2::uuid, 'run', $3::uuid, '{}'::jsonb)
		RETURNING event_id`,
		occurredAt, workspace, aggregate,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM outbox_events WHERE event_id = $1", id)
	})
	return id
}

func newAggregateID(t *testing.T, pool *pgxpool.Pool) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), "SELECT gen_random_uuid()").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func setNextDeliveryAt(t *testing.T, pool *pgxpool.Pool, eventID pgtype.UUID, offset string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE outbox_events SET next_delivery_at = now() + $2::interval WHERE event_id = $1", eventID, offset,
	); err != nil {
		t.Fatal(err)
	}
}

func releaseOutboxBackoff(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE outbox_events SET next_delivery_at = NULL WHERE next_delivery_at IS NOT NULL"); err != nil {
		t.Fatal(err)
	}
}

func outboxState(t *testing.T, pool *pgxpool.Pool, eventID pgtype.UUID) (attempts int, published, dead bool, backoffSeconds float64) {
	t.Helper()
	var wait *float64
	if err := pool.QueryRow(context.Background(), `
		SELECT delivery_attempts, published_at IS NOT NULL, dead_lettered_at IS NOT NULL,
		       extract(epoch FROM next_delivery_at - now())::float8
		FROM outbox_events WHERE event_id = $1`, eventID,
	).Scan(&attempts, &published, &dead, &wait); err != nil {
		t.Fatal(err)
	}
	if wait != nil {
		backoffSeconds = *wait
	}
	return
}

func failingFor(poison pgtype.UUID, deliveries *int) outbox.Handler {
	return func(_ context.Context, e outbox.Event) error {
		if e.AggregateID != poison {
			return nil
		}
		*deliveries++
		return errConsumerDown
	}
}

func TestAFailingEventDoesNotHoldBackLaterEventsOfOtherAggregates(t *testing.T) {
	pool := requireDB(t)
	f := newFixture(t, newAPI(t, pool), pool, "alice-outbox-hol")
	ws := mustUUID(t, f.workspaceID)
	poisonAgg, otherAgg := newAggregateID(t, pool), newAggregateID(t, pool)
	poison := insertPendingOutboxEvent(t, pool, ws, poisonAgg, "1990-01-01T00:00:00Z")
	behind := insertPendingOutboxEvent(t, pool, ws, otherAgg, "1990-01-02T00:00:00Z")

	var deliveries int
	w := &outbox.Worker{Pool: pool, Deliver: failingFor(poisonAgg, &deliveries)}
	n, err := w.Publish(context.Background())

	if !errors.Is(err, errConsumerDown) {
		t.Fatalf("Publish returned %v, want the delivery failure", err)
	}
	if n < 1 {
		t.Errorf("Publish reported %d published, want the event behind the failure counted", n)
	}
	if _, published, _, _ := outboxState(t, pool, behind); !published {
		t.Error("an event of another aggregate stayed unpublished behind a failing event")
	}
	if attempts, published, _, _ := outboxState(t, pool, poison); attempts != 1 || published {
		t.Errorf("failing event: attempts=%d published=%v, want 1 attempt and unpublished", attempts, published)
	}
}

func TestAFailingEventDoesNotHoldBackLaterEventsOfItsOwnAggregate(t *testing.T) {
	pool := requireDB(t)
	f := newFixture(t, newAPI(t, pool), pool, "alice-outbox-sameagg")
	ws := mustUUID(t, f.workspaceID)
	agg := newAggregateID(t, pool)
	first := insertPendingOutboxEvent(t, pool, ws, agg, "1990-01-01T00:00:00Z")
	second := insertPendingOutboxEvent(t, pool, ws, agg, "1990-01-02T00:00:00Z")

	failFirstOnly := func(_ context.Context, e outbox.Event) error {
		if e.EventID == first {
			return errConsumerDown
		}
		return nil
	}
	w := &outbox.Worker{Pool: pool, Deliver: failFirstOnly}
	if _, err := w.Publish(context.Background()); !errors.Is(err, errConsumerDown) {
		t.Fatalf("Publish returned %v, want the delivery failure", err)
	}

	if _, published, _, _ := outboxState(t, pool, second); !published {
		t.Error("a later event of the failing event's aggregate was held back; consumers do not depend on order")
	}
}

func TestAFailedEventIsNotClaimedAgainBeforeItsBackoffElapses(t *testing.T) {
	pool := requireDB(t)
	f := newFixture(t, newAPI(t, pool), pool, "alice-outbox-backoff")
	ws := mustUUID(t, f.workspaceID)
	agg := newAggregateID(t, pool)
	poison := insertPendingOutboxEvent(t, pool, ws, agg, "1990-01-01T00:00:00Z")

	var deliveries int
	w := &outbox.Worker{Pool: pool, Deliver: failingFor(agg, &deliveries)}
	_, _ = w.Publish(context.Background())
	_, _ = w.Publish(context.Background())
	if deliveries != 1 {
		t.Fatalf("event delivered %d times across two immediate passes, want 1 (second pass is inside the backoff)", deliveries)
	}

	setNextDeliveryAt(t, pool, poison, "1 hour")
	_, _ = w.Publish(context.Background())
	if deliveries != 1 {
		t.Errorf("event delivered %d times while its backoff runs another hour, want 1", deliveries)
	}

	setNextDeliveryAt(t, pool, poison, "-1 second")
	_, _ = w.Publish(context.Background())
	if deliveries != 2 {
		t.Errorf("event delivered %d times once its backoff elapsed, want 2", deliveries)
	}
}

func TestADeliveryFailureDoublesTheBackoffUpToTheCap(t *testing.T) {
	pool := requireDB(t)
	f := newFixture(t, newAPI(t, pool), pool, "alice-outbox-schedule")
	ws := mustUUID(t, f.workspaceID)

	for _, tc := range []struct {
		name            string
		attemptsBefore  int
		wantBackoffSecs float64
	}{
		{"first failure waits the base", 0, 5},
		{"second failure doubles it", 1, 10},
		{"seventh failure is the last below the cap", 6, 320},
		{"eighth failure reaches the cap", 7, 600},
		{"far past the cap stays at the cap", 29, 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := newAggregateID(t, pool)
			id := insertPendingOutboxEvent(t, pool, ws, agg, "1990-01-01T00:00:00Z")
			if _, err := pool.Exec(context.Background(),
				"UPDATE outbox_events SET delivery_attempts = $2 WHERE event_id = $1", id, tc.attemptsBefore); err != nil {
				t.Fatal(err)
			}
			var deliveries int
			w := &outbox.Worker{Pool: pool, MaxDeliveryAttempts: 1000, Deliver: failingFor(agg, &deliveries)}
			_, _ = w.Publish(context.Background())

			attempts, _, dead, wait := outboxState(t, pool, id)
			if dead || attempts != tc.attemptsBefore+1 {
				t.Fatalf("attempts=%d dead=%v, want %d attempts and not dead-lettered", attempts, dead, tc.attemptsBefore+1)
			}
			if wait > tc.wantBackoffSecs || wait < tc.wantBackoffSecs-3 {
				t.Errorf("next delivery in %.1fs, want %.0fs", wait, tc.wantBackoffSecs)
			}
		})
	}
}

func TestAnEventIsDeadLetteredOnlyAfterTheMaximumNumberOfFailedDeliveries(t *testing.T) {
	pool := requireDB(t)
	f := newFixture(t, newAPI(t, pool), pool, "alice-outbox-deadletter")
	ws := mustUUID(t, f.workspaceID)
	agg := newAggregateID(t, pool)
	poison := insertPendingOutboxEvent(t, pool, ws, agg, "1990-01-01T00:00:00Z")

	var deliveries int
	w := &outbox.Worker{Pool: pool, MaxDeliveryAttempts: 3, Deliver: failingFor(agg, &deliveries)}
	for failed := 1; failed <= 3; failed++ {
		releaseOutboxBackoff(t, pool)
		_, _ = w.Publish(context.Background())
		attempts, _, dead, _ := outboxState(t, pool, poison)
		if attempts != failed {
			t.Fatalf("after %d passes attempts = %d", failed, attempts)
		}
		if wantDead := failed == 3; dead != wantDead {
			t.Fatalf("after %d failures dead-lettered = %v, want %v", failed, dead, wantDead)
		}
	}

	releaseOutboxBackoff(t, pool)
	_, _ = w.Publish(context.Background())
	if deliveries != 3 {
		t.Errorf("event delivered %d times, want 3 (a dead-lettered event is never delivered again)", deliveries)
	}
}
