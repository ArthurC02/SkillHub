package readmodel

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type FinishedRun struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	FinishedAt  time.Time
}

func (r Reader) EvaluableRunsFinishedAfter(ctx context.Context, after FinishedRun, before time.Time, limit int32) ([]FinishedRun, error) {
	rows, err := r.queries().ListFinishedRunsBetween(ctx, gen.ListFinishedRunsBetweenParams{
		Statuses:       []string{string(gen.RunStatusSucceeded), string(gen.RunStatusFailed)},
		FinishedBefore: pgtype.Timestamptz{Time: before, Valid: true},
		CursorAt:       pgtype.Timestamptz{Time: after.FinishedAt, Valid: true},
		CursorID:       after.ID,
		PageSize:       limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]FinishedRun, len(rows))
	for i, row := range rows {
		out[i] = FinishedRun{ID: row.ID, WorkspaceID: row.WorkspaceID, FinishedAt: row.FinishedAt.Time}
	}
	return out, nil
}
