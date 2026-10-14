package run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

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

var passingRefusals = map[string]bool{
	ReasonWorkspaceConcurrency: true,
	ReasonScanUnavailable:      true,
}

func RefusalPasses(err error) bool {
	if errors.Is(err, ErrDispatchHalted) {
		return true
	}
	reason, refused := RefusalReason(err)
	return refused && passingRefusals[reason]
}
