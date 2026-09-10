package eval

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type RunEventConsumer struct {
	HasCurrentEvaluation func(context.Context, pgtype.UUID, pgtype.UUID) (bool, error)

	Insert func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

func (c *RunEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if event.EventType != outbox.RunSucceeded && event.EventType != outbox.RunFailed {
		return nil
	}

	if c.HasCurrentEvaluation == nil || c.Insert == nil {
		return errors.New("evaluation event consumer is not configured")
	}
	hasCurrent, err := c.HasCurrentEvaluation(ctx, event.WorkspaceID, event.AggregateID)
	if err != nil {
		return err
	}
	if hasCurrent {
		return nil
	}

	_, err = c.Insert(ctx, JobArgs{
		RunID:       pgconv.UUIDString(event.AggregateID),
		WorkspaceID: pgconv.UUIDString(event.WorkspaceID),
	}, InsertOpts())
	return err
}
