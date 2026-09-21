package eval

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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
	for _, row := range rows {
		if err := s.recoverEvaluation(ctx, row.WorkspaceID, row.ID, row.RunID); err != nil {
			return err
		}
	}
	_, err = s.RecoverLostSuggestionProvenance(ctx)
	return err
}
