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
// The destination is whatever the composition root wires into Deliver; with
// nothing wired it is the process log. This package stays generic (ADR-032 §1):
// it knows the delivery contract — ordered by commit time, at-least-once,
// idempotent marking — and nothing about who cares about which event.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
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
	// Deliver hands one event to its destination. Nil means the default log
	// delivery. A non-nil implementation must be safe to call twice with the same
	// event_id: this is at-least-once.
	Deliver func(context.Context, gen.OutboxEvent) error
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
	// One publisher owns the list/deliver/mark window. At-least-once still means
	// a crash after delivery may redeliver, but two healthy workers must not both
	// deliver the same unpublished snapshot concurrently.
	conn, err := w.Pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx,
		"SELECT pg_try_advisory_lock(hashtextextended('skillhub:outbox-publisher', 0))",
	).Scan(&locked); err != nil {
		return 0, err
	}
	if !locked {
		return 0, nil
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx),
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:outbox-publisher', 0))")
	}()

	q := gen.New(conn)

	// Retention first, and on every pass rather than only after a delivery: an
	// idle system still has last week's rows to drop, and "we publish nothing, so
	// nothing gets cleaned" is how a buffer quietly becomes an archive. The DELETE
	// is index-covered and touches only rows past the window, so a pass that finds
	// nothing costs one index probe.
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-w.retention()), Valid: true}
	if _, err := q.DeleteOutboxEventsPublishedBefore(ctx, cutoff); err != nil {
		return 0, err
	}

	events, err := q.ListUnpublishedOutboxEvents(ctx, publishBatch)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	deliver := w.Deliver
	if deliver == nil {
		deliver = logDelivery
	}

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
		if err := deliver(ctx, event); err != nil {
			if isolateErr := w.recordFailure(ctx, q, event, err); isolateErr != nil {
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
func (w *Worker) recordFailure(ctx context.Context, q *gen.Queries, event gen.OutboxEvent, cause error) error {
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
func logDelivery(_ context.Context, event gen.OutboxEvent) error {
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
