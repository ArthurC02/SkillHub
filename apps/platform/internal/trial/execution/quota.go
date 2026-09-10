package run

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

var errQuotaTransactionRequired = errors.New("run: quota transaction is required")

func (s *Service) requireQuota(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if tx == nil {
		return errQuotaTransactionRequired
	}
	q := gen.New(tx)
	reader := policy.UsageReader{
		WorkspaceCreatedAt: func(ctx context.Context, workspaceID pgtype.UUID) (time.Time, error) {
			return identity.ReadWorkspaceCreatedAt(ctx, tx, workspaceID)
		},
		CountRuns: quotaRunCounter(q),
	}
	reason, err := policy.EnforceQuota(ctx, reader, s.Quota, workspaceID)
	if err != nil {
		return refused(reason, err)
	}
	return nil
}

func (s *Service) QuotaFor(ctx context.Context, workspaceID pgtype.UUID) (policy.QuotaState, bool, error) {
	if !s.Quota.Enforced() {
		return policy.QuotaState{}, false, nil
	}
	reader := policy.UsageReader{
		WorkspaceCreatedAt: s.WorkspaceCreatedAt,
		CountRuns:          quotaRunCounter(s.queries()),
	}
	state, err := policy.Usage(ctx, reader, s.Quota, workspaceID, time.Now())
	if err != nil {
		return policy.QuotaState{}, false, err
	}

	state.Limits.Concurrent = MaxConcurrentRunsPerWorkspace
	return state, true, nil
}

func quotaRunCounter(q *gen.Queries) func(context.Context, pgtype.UUID, time.Time) (policy.RunUsage, error) {
	return func(ctx context.Context, workspaceID pgtype.UUID, since time.Time) (policy.RunUsage, error) {
		row, err := q.CountQuotaRuns(ctx, gen.CountQuotaRunsParams{
			WorkspaceID: workspaceID,
			Since:       pgtype.Timestamptz{Time: since, Valid: true},
		})
		if err != nil {
			return policy.RunUsage{}, err
		}
		var oldest *time.Time
		if row.Oldest.Valid {
			oldest = &row.Oldest.Time
		}
		return policy.RunUsage{Used: row.Used, Oldest: oldest}, nil
	}
}
