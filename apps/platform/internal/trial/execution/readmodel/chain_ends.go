package readmodel

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type ChainEnd struct {
	StartSnapshotID pgtype.UUID
	RunID           pgtype.UUID
	Status          string
	Terminal        bool
	Succeeded       bool
	Failed          bool
}

func (r Reader) ChainEnds(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]ChainEnd, error) {
	ends := make(map[pgtype.UUID]ChainEnd, len(runIDs))
	if len(runIDs) == 0 {
		return ends, nil
	}
	rows, err := r.queries().ListContinuationChainEnds(ctx, gen.ListContinuationChainEndsParams{
		WorkspaceID: workspaceID, RunIds: runIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		ends[row.StartID] = ChainEnd{
			StartSnapshotID: row.StartSnapshotID, RunID: row.EndID,
			Status: string(row.EndStatus), Terminal: run.IsTerminal(row.EndStatus),
			Succeeded: row.EndStatus == gen.RunStatusSucceeded, Failed: row.EndStatus == gen.RunStatusFailed,
		}
	}
	return ends, nil
}
