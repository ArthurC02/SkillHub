// Package outbox drains the transactional outbox (ADR-008, iron rule 9).
//
// The domain writes an event in the same transaction as the change it announces;
// this moves the committed rows onward. Splitting it that way is what makes "the
// run failed but nobody was told" impossible: the two facts cannot disagree,
// because they were one write.
//
// Delivery is at-least-once and deliberately so. An event is handed on *before* it
// is marked published, so a crash in between re-delivers rather than drops, and
// every consumer dedupes on event_id (ADR-008 一致性與冪等).
//
// The destination is whatever the composition root wires into Deliver, normally
// a Dispatcher fanning one event out to the consumers registered for its type.
// Wiring nothing is refused rather than defaulted to the log: an outbox that
// accepts every event and does nothing with it looks identical to a healthy one.
// This package stays generic (ADR-032 §1):
// it knows the delivery contract — ordered by commit time, at-least-once,
// idempotent marking — and nothing about who cares about which event.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	// DefaultPublishInterval is how often the backlog is drained. Domain events
	// here are low-volume state changes, not a trace stream.
	DefaultPublishInterval = 5 * time.Second
	// DefaultPublishedRetention is how long a delivered event stays in the buffer.
	// Long enough that an operator debugging last week's incident still finds the
	// event; short enough that the table stays the size of a week, not of the
	// product. The 400-day trail is audit_events, which is a different table for
	// exactly this reason.
	DefaultPublishedRetention = 7 * 24 * time.Hour
	// DefaultMaxDeliveryAttempts is how many failed deliveries an event gets before
	// it is isolated. Ten spans well over a minute of retries at the publish
	// interval, so a consumer that is merely restarting is never dead-lettered,
	// while one that is broken stops holding the queue open indefinitely.
	DefaultMaxDeliveryAttempts = 10
	// publishBatch bounds one pass. A backlog larger than this drains over several
	// passes rather than in one long transaction.
	publishBatch = 200
)

// PublishArgs carries nothing: the work is "whatever is unpublished".
type PublishArgs struct{}

func (PublishArgs) Kind() string { return "outbox_publish" }

// Worker drains the outbox on a schedule. Go is the only queue consumer
// (iron rule 7).
type Worker struct {
	river.WorkerDefaults[PublishArgs]
	Pool *pgxpool.Pool
	// Deliver hands one event to its destination, normally (*Dispatcher).Deliver.
	// It must be safe to call twice with the same event_id: this is at-least-once.
	//
	// Nil is a configuration error, not the log destination. It used to mean "log
	// it and mark it published", which reads as a harmless default and behaves as
	// a consumer that accepts everything and does nothing: a worker deployed with
	// its wiring dropped announced healthy delivery of events nobody acted on.
	// Wanting that behaviour is now something you have to say — see LogOnlyDelivery.
	Deliver Handler
	// LogOnlyDelivery drains to the process log instead of to consumers. For
	// development, where the point is to watch the events flow without standing up
	// what listens to them. In production it silently discards every reaction the
	// events were supposed to trigger, so it is an explicit field rather than the
	// consequence of forgetting one.
	LogOnlyDelivery bool
	// PublishInterval is how often the periodic job drains the backlog; zero means
	// DefaultPublishInterval. A field rather than a constant so a test can drain in
	// milliseconds instead of waiting out a production interval.
	PublishInterval time.Duration
	// PublishedRetention is how long a delivered event is kept before the publisher
	// deletes it; zero means DefaultPublishedRetention. Fields for the same reason
	// as above: a test proves retention in one pass rather than in a week.
	PublishedRetention time.Duration
	// MaxDeliveryAttempts is how many failures one event gets before it is
	// dead-lettered; zero means DefaultMaxDeliveryAttempts.
	MaxDeliveryAttempts int
}

// Interval is the period this worker should be registered with. Consult it at
// the composition root: River takes the interval when the periodic job is
// registered, not from the worker.
func (w *Worker) Interval() time.Duration {
	if w.PublishInterval > 0 {
		return w.PublishInterval
	}
	return DefaultPublishInterval
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[PublishArgs]) error {
	_, err := w.Publish(ctx)
	return err
}

// Publish drains up to one batch and returns how many events were handed on.
// Exported so tests and one-off tooling can drain without waiting for a timer.
func (w *Worker) Publish(ctx context.Context) (int, error) {
	// Before the lock and before the pool: a worker with no destination cannot
	// publish anything, and finding that out after reading a batch would mean
	// having decided what to do with events we have no way to hand on.
	deliver, err := w.delivery()
	if err != nil {
		return 0, err
	}

	events, err := w.claim(ctx)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	// Past this point the advisory lock is released and the pool connection it
	// was held on is back. Everything below talks to the pool, and that is the
	// whole point: see claim's comment for what holding it cost.
	q := gen.New(w.Pool)

	// Delivered one at a time, marked in one batch. A failure stops the pass: the
	// events behind it stay unpublished and get re-delivered, which is the
	// at-least-once guarantee doing its job. The prefix that did land is still
	// marked — re-delivering it would be legal but pointless.
	//
	// The failure is both logged and returned. Returned is what makes River retry
	// the publish job instead of the backlog sitting there until the next tick;
	// logged is what makes a consumer that fails every time visible rather than a
	// silent stall.
	//
	// And it is counted, which is what stops "stops the pass" from meaning "stops
	// the outbox". Before this, one event a consumer could never accept held every
	// event committed after it, for as long as the consumer stayed broken — the
	// unbounded resend ADR-008 forbids. Now the count lives on the row, so it
	// survives the publisher that did the counting, and the event steps aside once
	// it is spent (ADR-008 Poison Message).
	var failure error
	ids := make([]pgtype.UUID, 0, len(events))
	for _, event := range events {
		owned := eventFromRow(event)
		if err := deliver(ctx, owned); err != nil {
			if isolateErr := w.recordFailure(ctx, q, owned, err); isolateErr != nil {
				// Reported instead of the delivery error: a publisher that cannot
				// write the attempt count cannot make progress towards isolation
				// either, and that is the more urgent of the two facts.
				failure = isolateErr
				break
			}
			failure = fmt.Errorf("deliver %s (%s): %w", pgconv.UUIDString(event.EventID), event.EventType, err)
			break
		}
		ids = append(ids, event.EventID)
	}
	if len(ids) == 0 {
		return 0, failure
	}
	if _, err := q.MarkOutboxEventsPublished(ctx, ids); err != nil {
		return 0, err
	}
	return len(ids), failure
}

// claim takes the publisher lock, does the pass's housekeeping, and returns the
// batch to hand on — then gives the connection back BEFORE anything is
// delivered.
//
// That last clause is the reason this is a separate function. The lock is a
// session lock, so it lives on one connection and holding it means holding that
// connection; the previous version held it across the whole pass, delivery
// included. In every deployment with a pool of one — which is clean mode, and
// clean mode by definition also runs the consumers in this same process
// (ADR-060 決策 6) — the first thing a consumer does is ask that same pool for a
// connection, and there is none: the publisher is holding it and is waiting for
// the consumer. m6/report-inmemory-postgres.md had already measured this exact
// shape at 238 seconds before it grew back here, one package over.
//
// What releasing the lock early gives up, stated rather than hidden: two
// publishers can now list overlapping batches and deliver the same event twice.
// That is not a regression, it is the contract this package's doc comment opens
// with — delivery is at-least-once and every consumer dedupes on event_id
// (ADR-008). The lock still does the job it is genuinely needed for, which is
// keeping two publishers from interleaving the retention DELETE and the list.
// The alternative — a second connection reserved for delivery — buys exactly
// nothing the consumers do not already provide, and buys it by requiring a
// second connection, which is the resource clean mode does not have.
//
// The test that goes red without the early release is
// TestCleanModeDrainsTheOutboxOnOneConnection, in entrypoint/api/apiserver. It
// fails by timing out rather than by asserting a wrong value, which is the only
// way a deadlock ever fails, and it needs the whole process shape to see it — so
// it is over there with the composition root and not in this package (04 丙-88).
func (w *Worker) claim(ctx context.Context) ([]gen.OutboxEvent, error) {
	conn, err := w.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx,
		"SELECT pg_try_advisory_lock(hashtextextended('skillhub:outbox-publisher', 0))",
	).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, nil
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx),
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:outbox-publisher', 0))")
	}()

	q := gen.New(conn)

	// Republished every pass, on the connection that already holds the publisher
	// lock, so exactly one process reports it and the series does not flap
	// between workers. It is a level and not a rate: a dead-lettered event is a
	// committed domain fact nobody will ever be told about, and it stays in the
	// table until a human decides what to do, so what an operator needs to see is
	// that it is still there (see metrics.OutboxDeadLetteredCurrent).
	//
	// A read failure leaves the gauge alone rather than zeroing it: "we could not
	// count" must never render as "there is nothing in quarantine". The pass
	// carries on, because the count is bookkeeping and draining the backlog is
	// the job.
	if n, err := q.CountDeadLetteredOutboxEvents(ctx); err != nil {
		slog.Warn("outbox: dead-letter count unavailable; the gauge keeps its last value", "error", err)
	} else {
		metrics.OutboxDeadLetteredCurrent.Set(float64(n))
	}

	// Retention first, and on every pass rather than only after a delivery: an
	// idle system still has last week's rows to drop, and "we publish nothing, so
	// nothing gets cleaned" is how a buffer quietly becomes an archive. The DELETE
	// is index-covered and touches only rows past the window, so a pass that finds
	// nothing costs one index probe.
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-w.retention()), Valid: true}
	if _, err := q.DeleteOutboxEventsPublishedBefore(ctx, cutoff); err != nil {
		return nil, err
	}

	return q.ListUnpublishedOutboxEvents(ctx, publishBatch)
}

// delivery picks the destination, or refuses to run without one. Separate from
// Publish so the refusal is testable without a database: "fail closed" is a
// claim about a decision, and the decision is here.
func (w *Worker) delivery() (Handler, error) {
	if w.Deliver != nil {
		return w.Deliver, nil
	}
	if w.LogOnlyDelivery {
		return logDelivery, nil
	}
	return nil, errors.New("outbox worker has no Deliver and LogOnlyDelivery is off: refusing to mark events published that nobody consumed")
}

func (w *Worker) retention() time.Duration {
	if w.PublishedRetention > 0 {
		return w.PublishedRetention
	}
	return DefaultPublishedRetention
}

func (w *Worker) maxAttempts() int32 {
	if w.MaxDeliveryAttempts > 0 {
		return int32(w.MaxDeliveryAttempts)
	}
	return DefaultMaxDeliveryAttempts
}

// recordFailure counts one failed delivery and, at the threshold, isolates the
// event so the backlog behind it can move.
//
// Outside the publisher's advisory-locked pass in transaction terms but not in
// exclusion terms: there is no transaction here on purpose. A count that rolled
// back alongside the failure it counts would never reach the threshold.
//
// Isolation is loud and is not a repair. The row is left in place for a human:
// nothing here replays it, and nothing here decides that an undeliverable event
// did not matter. The metric is the alert surface ADR-008 asks for; the Prometheus
// rule that watches it belongs to O11Y-PROMTOOL-001, not to this package.
func (w *Worker) recordFailure(ctx context.Context, q *gen.Queries, event Event, cause error) error {
	row, err := q.RecordOutboxDeliveryFailure(ctx, gen.RecordOutboxDeliveryFailureParams{
		EventID:     event.EventID,
		MaxAttempts: w.maxAttempts(),
	})
	if err != nil {
		return fmt.Errorf("record delivery failure for %s: %w", pgconv.UUIDString(event.EventID), err)
	}
	if !row.DeadLetteredAt.Valid {
		slog.Error("domain event delivery failed",
			"event_id", pgconv.UUIDString(event.EventID),
			"event_type", event.EventType,
			"delivery_attempts", row.DeliveryAttempts,
			"error", cause)
		return nil
	}
	metrics.OutboxDeadLettered.WithLabelValues(event.EventType).Inc()
	slog.Error("domain event dead-lettered: delivery failed too many times, the event is now isolated and needs a human",
		"event_id", pgconv.UUIDString(event.EventID),
		"event_type", event.EventType,
		"correlation_id", pgconv.UUIDString(event.CorrelationID),
		"delivery_attempts", row.DeliveryAttempts,
		"error", cause)
	return nil
}

// logDelivery is the MVP destination. The payload is written out because it is
// the event; the writers are the ones that keep it to identifiers and outcome
// (iron rule 11), and an outbox that strips its own payload would prove nothing
// about the transport it stands in for.
func logDelivery(_ context.Context, event Event) error {
	slog.Info("domain event published",
		"event_id", pgconv.UUIDString(event.EventID),
		"event_type", event.EventType,
		"event_version", event.EventVersion,
		"correlation_id", pgconv.UUIDString(event.CorrelationID),
		"aggregate_type", event.AggregateType,
		"aggregate_id", pgconv.UUIDString(event.AggregateID),
		"payload", string(event.Payload),
	)
	return nil
}
