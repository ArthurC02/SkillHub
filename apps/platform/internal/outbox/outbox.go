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
)

const (
	// DefaultPublishInterval is how often the backlog is drained. Domain events
	// here are low-volume state changes, not a trace stream.
	DefaultPublishInterval = 5 * time.Second
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
	// silent stall (contracts/events/domain-events.md §5 still owes a DLQ).
	var failure error
	ids := make([]pgtype.UUID, 0, len(events))
	for _, event := range events {
		if err := deliver(ctx, event); err != nil {
			slog.Error("domain event delivery failed",
				"event_id", uuidString(event.EventID),
				"event_type", event.EventType,
				"error", err)
			failure = fmt.Errorf("deliver %s (%s): %w", uuidString(event.EventID), event.EventType, err)
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

// logDelivery is the MVP destination. The payload is written out because it is
// the event; the writers are the ones that keep it to identifiers and outcome
// (iron rule 11), and an outbox that strips its own payload would prove nothing
// about the transport it stands in for.
func logDelivery(_ context.Context, event gen.OutboxEvent) error {
	slog.Info("domain event published",
		"event_id", uuidString(event.EventID),
		"event_type", event.EventType,
		"event_version", event.EventVersion,
		"correlation_id", uuidString(event.CorrelationID),
		"aggregate_type", event.AggregateType,
		"aggregate_id", uuidString(event.AggregateID),
		"payload", string(event.Payload),
	)
	return nil
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
