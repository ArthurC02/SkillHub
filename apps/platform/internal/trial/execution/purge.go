package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (s *Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.ClearSightings == nil {
		return errReconcilePersistenceNotConfigured
	}
	ids, err := gen.New(tx).DeleteWorkspaceRunArtifacts(ctx, workspaceID)
	if err != nil {
		return err
	}
	return s.ClearSightings(ctx, tx, ids)
}

func (*Service) WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceRunArtifactObjectKeys(ctx, workspaceID)
}

func (*Service) PurgeQuiescent(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
	return gen.New(db).AccountPurgeReady(ctx, workspaceID)
}

func (*Service) SkillVersionsInRuns(ctx context.Context, db gen.DBTX, versionIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListSkillVersionsInRuns(ctx, versionIDs)
}
