package credit

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// preSplitStoreA and preSplitStoreB independently spell out every method
// Store exposed before the split; preSplitStore below checks that set
// against Store in both directions.
type preSplitStoreA interface {
	Balance(ctx context.Context, tx DBTX, userID pgtype.UUID) (int64, error)
	RecordCostEvent(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error)
	CostEventExists(ctx context.Context, tx DBTX, idempotencyKey string) (bool, error)
	ApplyDebit(ctx context.Context, tx DBTX, d DebitEntry) (balanceAfter int64, existed bool, err error)
	ApplyGrant(ctx context.Context, tx DBTX, g GrantEntry) (balanceAfter int64, applied bool, err error)
	RecentStatistics(ctx context.Context, kind CostKind) (Statistics, error)
	RecomputeStatistics(ctx context.Context, kind CostKind, windowStart, windowEnd time.Time) (Statistics, error)
	SummarizeSession(ctx context.Context, tx DBTX, sessionID pgtype.UUID) error
	SweepSessionSummaries(ctx context.Context, windowStart, idleBefore time.Time) (int64, error)
	RecentEntries(ctx context.Context, tx DBTX, userID pgtype.UUID, limit int32) ([]LedgerEntry, error)
}

type preSplitStoreB interface {
	OwnEntries(ctx context.Context, userID pgtype.UUID, page EntryPage) ([]StatementEntry, error)
	LatestStatistics(ctx context.Context) ([]KindStatistics, error)
	DailyCost(ctx context.Context, since time.Time) ([]DailyAmount, error)
	DailyCredits(ctx context.Context, since time.Time) ([]DailyAmount, error)
	BalanceTotal(ctx context.Context) (int64, error)
}

type preSplitStore interface {
	preSplitStoreA
	preSplitStoreB
}

var (
	_ preSplitStore = Store(nil)
	_ Store         = preSplitStore(nil)
)
