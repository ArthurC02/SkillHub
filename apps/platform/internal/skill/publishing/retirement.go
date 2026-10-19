package publishing

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type SkillDeletedConsumer struct {
	Pool *pgxpool.Pool
}

func (c *SkillDeletedConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	_, err := gen.New(c.Pool).RetireSkillPublication(ctx, gen.RetireSkillPublicationParams{
		WorkspaceID: event.WorkspaceID, SkillID: event.AggregateID, RetiredAt: event.OccurredAt,
	})
	return err
}
