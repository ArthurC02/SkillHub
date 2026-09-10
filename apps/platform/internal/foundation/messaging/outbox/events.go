package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Event struct {
	EventID          pgtype.UUID
	EventType        string
	EventVersion     int32
	OccurredAt       pgtype.Timestamptz
	CorrelationID    pgtype.UUID
	CausationID      pgtype.UUID
	WorkspaceID      pgtype.UUID
	AggregateType    string
	AggregateID      pgtype.UUID
	Payload          []byte
	PublishedAt      pgtype.Timestamptz
	DeliveryAttempts int32
	DeadLetteredAt   pgtype.Timestamptz
}

type NewEvent struct {
	EventType     string
	EventVersion  int32
	CorrelationID pgtype.UUID
	CausationID   pgtype.UUID
	WorkspaceID   pgtype.UUID
	AggregateType string
	AggregateID   pgtype.UUID
	Payload       []byte
}

func eventFromRow(row gen.OutboxEvent) Event {
	return Event{
		EventID: row.EventID, EventType: row.EventType, EventVersion: row.EventVersion,
		OccurredAt: row.OccurredAt, CorrelationID: row.CorrelationID, CausationID: row.CausationID,
		WorkspaceID: row.WorkspaceID, AggregateType: row.AggregateType, AggregateID: row.AggregateID,
		Payload: row.Payload, PublishedAt: row.PublishedAt, DeliveryAttempts: row.DeliveryAttempts,
		DeadLetteredAt: row.DeadLetteredAt,
	}
}

const (
	RunQueued       = "run.queued"
	RunProvisioning = "run.provisioning"
	RunPreparing    = "run.preparing"
	RunRunning      = "run.running"
	RunEvaluating   = "run.evaluating"
	RunSucceeded    = "run.succeeded"
	RunFailed       = "run.failed"
	RunCancelled    = "run.cancelled"
	RunTimedOut     = "run.timed_out"
)

const (
	RunCleanupCleaned = "run.cleanup_cleaned"
	RunCleanupFailed  = "run.cleanup_failed"
)

const (
	AggregateRun = "run"

	EventVersion1 = int32(1)
)

var EventTypes = []string{
	RunQueued,
	RunProvisioning,
	RunPreparing,
	RunRunning,
	RunEvaluating,
	RunSucceeded,
	RunFailed,
	RunCancelled,
	RunTimedOut,
	RunCleanupCleaned,
	RunCleanupFailed,
}

func StatusEvent(status string) (string, error) {
	switch status {
	case "queued":
		return RunQueued, nil
	case "provisioning":
		return RunProvisioning, nil
	case "preparing":
		return RunPreparing, nil
	case "running":
		return RunRunning, nil
	case "evaluating":
		return RunEvaluating, nil
	case "succeeded":
		return RunSucceeded, nil
	case "failed":
		return RunFailed, nil
	case "cancelled":
		return RunCancelled, nil
	case "timed_out":
		return RunTimedOut, nil
	}
	return "", fmt.Errorf("no domain event for run status %q", status)
}

func CleanupEvent(status string) (string, error) {
	switch status {
	case "cleaned":
		return RunCleanupCleaned, nil
	case "failed":
		return RunCleanupFailed, nil
	}
	return "", fmt.Errorf("no domain event for cleanup status %q", status)
}

func Insert(ctx context.Context, tx pgx.Tx, event NewEvent) error {
	if tx == nil {
		return fmt.Errorf("outbox: transaction is not configured")
	}
	_, err := gen.New(tx).InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType: event.EventType, EventVersion: event.EventVersion,
		CorrelationID: event.CorrelationID, CausationID: event.CausationID,
		WorkspaceID: event.WorkspaceID, AggregateType: event.AggregateType,
		AggregateID: event.AggregateID, Payload: event.Payload,
	})
	return err
}
