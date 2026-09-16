package testlab

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceDatasetObjectKeys(ctx, workspaceID)
}

func (s *Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.ClearSightings == nil {
		return errPersistenceNotConfigured
	}
	q := gen.New(tx)
	ids, err := q.DeleteWorkspaceDatasets(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.ClearSightings(ctx, tx, ids); err != nil {
		return err
	}
	testCases, err := q.ListWorkspaceTestCaseIDs(ctx, workspaceID)
	if err != nil {
		return err
	}
	snapshotted, err := q.ListSnapshottedTestCases(ctx, testCases)
	if err != nil {
		return err
	}
	erasable := testCasesNoRunFroze(testCases, snapshotted)
	if len(erasable) == 0 {
		return nil
	}
	_, err = q.DeleteWorkspaceTestCases(ctx, gen.DeleteWorkspaceTestCasesParams{WorkspaceID: workspaceID, TestCaseIds: erasable})
	return err
}

func testCasesNoRunFroze(testCases, snapshotted []pgtype.UUID) []pgtype.UUID {
	var erasable []pgtype.UUID
	for _, id := range testCases {
		if !slices.Contains(snapshotted, id) {
			erasable = append(erasable, id)
		}
	}
	return erasable
}

func SkillsWithTestCases(ctx context.Context, db gen.DBTX, skillIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListSkillsWithTestCases(ctx, skillIDs)
}
