package batch

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type BatchAdvanceArgs struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
}

func (BatchAdvanceArgs) Kind() string { return "advance_evaluation_batches" }

type BatchEventConsumer struct {
	HasUnsettledBatch func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)

	Enqueue func(context.Context, BatchAdvanceArgs) error
}

func (c *BatchEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if c.HasUnsettledBatch == nil || c.Enqueue == nil {
		return errors.New("evaluation batch event consumer is not configured")
	}
	unsettled, err := c.HasUnsettledBatch(ctx, event.WorkspaceID)
	if err != nil || !unsettled {
		return err
	}
	return c.Enqueue(ctx, BatchAdvanceArgs{WorkspaceID: pgconv.UUIDString(event.WorkspaceID)})
}
