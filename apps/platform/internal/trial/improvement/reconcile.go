package eval

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	RecoveryInterval   = 5 * time.Minute
	RecoveryStaleAfter = 10 * time.Minute

	recoveryBatch = 100
)

func statusesAwaitingTheJudge() []string {
	var awaiting []string
	for _, status := range AllStatuses() {
		if status.AwaitsTheJudge() {
			awaiting = append(awaiting, string(status))
		}
	}
	return awaiting
}

func (s *Service) RecoverPending(ctx context.Context) error {
	rows, err := s.queries().ListStalePendingEvaluations(ctx, gen.ListStalePendingEvaluationsParams{
		AwaitingStatuses: statusesAwaitingTheJudge(),
		StaleBefore:      pgtype.Timestamptz{Time: time.Now().Add(-RecoveryStaleAfter), Valid: true},
		ResultLimit:      recoveryBatch,
	})
	if err != nil {
		return err
	}
	var failed []error
	for _, row := range rows {
		if err := s.recoverEvaluation(ctx, row.WorkspaceID, row.ID, row.RunID); err != nil {
			slog.Warn("a stale evaluation was not recovered; the rest of the batch still is",
				"evaluation_id", pgconv.UUIDString(row.ID), "error", err)
			failed = append(failed, err)
		}
	}
	_, err = s.RecoverLostSuggestionProvenance(ctx)
	return errors.Join(append(failed, err)...)
}
