package eval

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type RunEventConsumer struct {
	HasCurrentEvaluation func(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error)

	Enqueue func(context.Context, JobArgs) error
}

func (c *RunEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if event.EventType != outbox.RunSucceeded && event.EventType != outbox.RunFailed {
		return nil
	}

	if c.HasCurrentEvaluation == nil || c.Enqueue == nil {
		return errors.New("evaluation event consumer is not configured")
	}
	hasCurrent, err := c.HasCurrentEvaluation(ctx, event.WorkspaceID, event.AggregateID)
	if err != nil {
		return err
	}
	if hasCurrent {
		return nil
	}

	err = c.Enqueue(ctx, JobArgs{
		RunID:       pgconv.UUIDString(event.AggregateID),
		WorkspaceID: pgconv.UUIDString(event.WorkspaceID),
	})
	return err
}

type BatchEventConsumer struct {
	HasLaunchingBatch func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)

	Enqueue func(context.Context, BatchAdvanceArgs) error
}

func (c *BatchEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if c.HasLaunchingBatch == nil || c.Enqueue == nil {
		return errors.New("evaluation batch event consumer is not configured")
	}
	launching, err := c.HasLaunchingBatch(ctx, event.WorkspaceID)
	if err != nil || !launching {
		return err
	}
	return c.Enqueue(ctx, BatchAdvanceArgs{WorkspaceID: pgconv.UUIDString(event.WorkspaceID)})
}
