package run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ChainEnd struct {
	StartSnapshotID pgtype.UUID
	RunID           pgtype.UUID
	Status          string
	Terminal        bool
}

func (s *Service) ChainEnds(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]ChainEnd, error) {
	ends := make(map[pgtype.UUID]ChainEnd, len(runIDs))
	if len(runIDs) == 0 {
		return ends, nil
	}
	rows, err := s.queries().ListContinuationChainEnds(ctx, gen.ListContinuationChainEndsParams{
		WorkspaceID: workspaceID, RunIds: runIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		ends[row.StartID] = ChainEnd{
			StartSnapshotID: row.StartSnapshotID, RunID: row.EndID,
			Status: string(row.EndStatus), Terminal: IsTerminal(row.EndStatus),
		}
	}
	return ends, nil
}

func (s *Service) ConfirmedPermissionHash(
	ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID,
) (hash string, confirmed bool, err error) {
	summary, err := s.PermissionSummaryFor(ctx, workspaceID, skillID, versionID, testCaseID)
	if err != nil {
		return "", false, err
	}
	_, err = s.queries().GetRunPermissionConfirmation(ctx, gen.GetRunPermissionConfirmationParams{
		WorkspaceID: workspaceID, SkillVersionID: versionID, TestCaseID: testCaseID, SummaryHash: summary.Hash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return summary.Hash, false, nil
	}
	return summary.Hash, err == nil, err
}

func RefusalReason(err error) (string, bool) {
	r, isRefusal := errors.AsType[refusal](err)
	return r.reason, isRefusal
}
