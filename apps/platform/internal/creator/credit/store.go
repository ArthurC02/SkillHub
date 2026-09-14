package credit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var ErrNoStatistics = errors.New("credit: no statistics recorded yet")

type CostKind string

const (
	KindCreationStep    CostKind = "creation_step"
	KindSearchEmbedding CostKind = "search_embedding"
	KindIndexEnrich     CostKind = "index_enrich"
	KindReview          CostKind = "review"
	KindSuggestion      CostKind = "suggestion"
	KindGenerate        CostKind = "generate"
	KindMatchReasons    CostKind = "match_reasons"

	KindRun CostKind = "run"

	KindCreationSession CostKind = "creation_session"
)

func AllCostEventKinds() []CostKind {
	return []CostKind{
		KindCreationStep, KindSearchEmbedding, KindIndexEnrich, KindReview,
		KindSuggestion, KindGenerate, KindMatchReasons, KindRun,
	}
}

func AllStatisticKinds() []CostKind {
	return []CostKind{
		KindCreationStep, KindSearchEmbedding, KindIndexEnrich, KindReview,
		KindSuggestion, KindGenerate, KindMatchReasons, KindRun, KindCreationSession,
	}
}

const (
	EntryDebit      = "debit"
	EntryGrant      = "grant"
	EntryTopup      = "topup"
	EntryAdjustment = "adjustment"
)

const (
	RefCreationSession = "creation_session"
	RefRun             = "run"
	RefSkillVersion    = "skill_version"
	RefOperatorGrant   = "operator_grant"
)

type CostEvent struct {
	Kind             CostKind
	Model            string
	PromptVersion    string
	PromptTokens     int64
	CompletionTokens int64
	UsdMicros        int64

	Estimated bool

	WorkspaceID    pgtype.UUID
	UserID         pgtype.UUID
	RefType        string
	RefID          pgtype.UUID
	IdempotencyKey string
}

type DebitEntry struct {
	CostEventID string
	UserID      pgtype.UUID
	Credits     int64

	UsdMicros      int64
	MarkupBps      int64
	Estimated      bool
	RefType        string
	RefID          pgtype.UUID
	IdempotencyKey string
}

type GrantEntry struct {
	UserID         pgtype.UUID
	EntryKind      string
	Credits        int64
	Reason         string
	OperatorID     pgtype.UUID
	IdempotencyKey string
}

type Store interface {
	Balance(ctx context.Context, tx DBTX, userID pgtype.UUID) (int64, error)

	RecordCostEvent(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error)

	ApplyDebit(ctx context.Context, tx DBTX, d DebitEntry) (balanceAfter int64, existed bool, err error)

	ApplyGrant(ctx context.Context, tx DBTX, g GrantEntry) (balanceAfter int64, err error)

	RecentStatistics(ctx context.Context, kind CostKind) (Statistics, error)

	RecomputeStatistics(ctx context.Context, kind CostKind, windowStart, windowEnd time.Time) (Statistics, error)

	SummarizeSession(ctx context.Context, tx DBTX, sessionID pgtype.UUID) error

	SweepSessionSummaries(ctx context.Context, windowStart, idleBefore time.Time) (int64, error)

	RecentEntries(ctx context.Context, tx DBTX, userID pgtype.UUID, limit int32) ([]LedgerEntry, error)

	LatestStatistics(ctx context.Context) ([]KindStatistics, error)

	DailyCost(ctx context.Context, since time.Time) ([]DailyAmount, error)

	DailyCredits(ctx context.Context, since time.Time) ([]DailyAmount, error)

	BalanceTotal(ctx context.Context) (int64, error)
}

type LedgerEntry struct {
	Kind         string
	DeltaCredits int64
	RefType      *string
	Estimated    bool
	CreatedAt    time.Time
}

type KindStatistics struct {
	Kind         CostKind
	WindowStart  time.Time
	WindowEnd    time.Time
	SampleCount  int64
	P50UsdMicros *int64
	P90UsdMicros *int64
	P95UsdMicros *int64
	MaxUsdMicros *int64
}

type DailyAmount struct {
	Day   time.Time
	Key   string
	Count int64
	Total int64
}
