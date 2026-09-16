package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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

func WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceRunArtifactObjectKeys(ctx, workspaceID)
}

func PurgeQuiescent(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
	return gen.New(db).AccountPurgeReady(ctx, accountPurgeReadiness(workspaceID))
}

func accountPurgeReadiness(workspaceID pgtype.UUID) gen.AccountPurgeReadyParams {
	return gen.AccountPurgeReadyParams{
		WorkspaceID:           workspaceID,
		TerminalStatuses:      terminalStatuses(),
		SettledCleanupStatus:  string(gen.RunCleanupStatusCleaned),
		UnprovableGrantStates: []string{string(ObjectGrantStateLegacyUnknown)},
		ClockTolerance:        pgconv.Interval(purgeClockTolerance),
	}
}

func terminalStatuses() []string {
	var terminal []string
	for _, status := range AllStatuses {
		if IsTerminal(status) {
			terminal = append(terminal, string(status))
		}
	}
	return terminal
}

func SkillVersionsInRuns(ctx context.Context, db gen.DBTX, versionIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListSkillVersionsInRuns(ctx, versionIDs)
}
