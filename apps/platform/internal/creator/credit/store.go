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

const (
	KindCreationStep    = "creation_step"
	KindSearchEmbedding = "search_embedding"
	KindIndexEnrich     = "index_enrich"
	KindReview          = "review"
	KindSuggestion      = "suggestion"
	KindGenerate        = "generate"
	KindMatchReasons    = "match_reasons"

	KindRun = "run"

	KindCreationSession = "creation_session"
)

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
	Kind             string
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

	RecentStatistics(ctx context.Context, kind string) (Statistics, error)

	RecomputeStatistics(ctx context.Context, kind string, windowStart, windowEnd time.Time) (Statistics, error)

	SummarizeSession(ctx context.Context, tx DBTX, sessionID pgtype.UUID) error

	SweepSessionSummaries(ctx context.Context, windowStart, idleBefore time.Time) (int64, error)
}
