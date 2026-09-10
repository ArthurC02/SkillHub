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
	DefaultPublishInterval = 5 * time.Second

	DefaultPublishedRetention = 7 * 24 * time.Hour

	DefaultMaxDeliveryAttempts = 10

	publishBatch = 200
)

type PublishArgs struct{}

func (PublishArgs) Kind() string { return "outbox_publish" }

type Worker struct {
	river.WorkerDefaults[PublishArgs]
	Pool *pgxpool.Pool

	Deliver Handler

	LogOnlyDelivery bool

	PublishInterval time.Duration

	PublishedRetention time.Duration

	MaxDeliveryAttempts int
}

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

func (w *Worker) Publish(ctx context.Context) (int, error) {

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

	q := gen.New(w.Pool)

	var failure error
	ids := make([]pgtype.UUID, 0, len(events))
	for _, event := range events {
		owned := eventFromRow(event)
		if err := deliver(ctx, owned); err != nil {
			if isolateErr := w.recordFailure(ctx, q, owned, err); isolateErr != nil {

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

// claim acquires the advisory lock on its own pooled connection and releases
// that connection before returning, so a delivery step needing a connection
// from the same pool never blocks waiting on the lock holder.
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

	if n, err := q.CountDeadLetteredOutboxEvents(ctx); err != nil {
		slog.Warn("outbox: dead-letter count unavailable; the gauge keeps its last value", "error", err)
	} else {
		metrics.OutboxDeadLetteredCurrent.Set(float64(n))
	}

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-w.retention()), Valid: true}
	if _, err := q.DeleteOutboxEventsPublishedBefore(ctx, cutoff); err != nil {
		return nil, err
	}

	return q.ListUnpublishedOutboxEvents(ctx, publishBatch)
}

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
