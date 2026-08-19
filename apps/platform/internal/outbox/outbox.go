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
// The MVP destination is the process log. There is no consumer yet — the Run
// workflow's reader is the platform itself, through the database — and inventing a
// broker before something needs to subscribe would be infrastructure with no
// subscriber. What this package does own, and what the tests pin down, is the
// delivery contract: ordered by commit time, at-least-once, idempotent marking.
// Swapping the log for a real transport is one function.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

const (
	// PublishInterval is how often the backlog is drained. Domain events here are
	// low-volume state changes, not a trace stream.
	PublishInterval = 5 * time.Second
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
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[PublishArgs]) error {
	_, err := w.Publish(ctx)
	return err
}

// Publish drains up to one batch and returns how many events were handed on.
// Exported so tests and one-off tooling can drain without waiting for a timer.
func (w *Worker) Publish(ctx context.Context) (int, error) {
	q := gen.New(w.Pool)
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

	// Delivered one at a time, marked in one batch. A failure part way through
	// leaves the rest unpublished and the successful ones unmarked, so the next
	// pass re-delivers them — which is the at-least-once guarantee doing its job,
	// not a bug to fix by marking first.
	ids := make([]pgtype.UUID, 0, len(events))
	for _, event := range events {
		if err := deliver(ctx, event); err != nil {
			break
		}
		ids = append(ids, event.EventID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err := q.MarkOutboxEventsPublished(ctx, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
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
