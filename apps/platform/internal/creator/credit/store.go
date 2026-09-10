package credit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// DBTX is the smallest database handle credit's write methods need — the
// same shape audit.DBTX and sqlc's generated Queries use, so a caller's
// existing pgx.Tx (which has strictly more methods) satisfies it with no
// wrapping, and a test can satisfy it with a two-method fake with no
// database at all.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ErrNoStatistics is returned by Store.RecentStatistics when no
// cost_statistics row exists yet for a kind — distinct from a Statistics
// zero value so gate ①'s fallback (decision 8) is a deliberate branch, not
// something that happens to fall out of an empty struct.
var ErrNoStatistics = errors.New("credit: no statistics recorded yet")

// Cost-event / cost-statistics kinds (migration 0060's CHECK constraint,
// ADR-068 decision 3). Every paid call this platform makes is recorded
// under exactly one of these.
const (
	KindCreationStep    = "creation_step"
	KindSearchEmbedding = "search_embedding"
	KindIndexEnrich     = "index_enrich"
	KindReview          = "review"
	KindSuggestion      = "suggestion"
	KindGenerate        = "generate"
	// KindRun is a trial Run's gateway spend; unlike the other kinds it has a
	// platform-set ceiling (the Virtual Key's max_budget).
	KindRun = "run"
)

// credit_entries.kind values (decision 3).
const (
	EntryDebit      = "debit"
	EntryGrant      = "grant"
	EntryTopup      = "topup"
	EntryAdjustment = "adjustment"
)

// ref_type values shared by cost_events and credit_entries. RefOperatorGrant
// is credit_entries-only (a grant/adjustment has no cost_events row behind
// it, so it names the operator action instead of a session/run/version).
const (
	RefCreationSession = "creation_session"
	RefRun             = "run"
	RefSkillVersion    = "skill_version"
	RefOperatorGrant   = "operator_grant"
)

// CostEvent is one row of the platform's real-spend ledger (migration
// 0060's cost_events). It never holds query text (ADR-029's "no query
// content" discipline, applied to cost the same way it applies to
// analytics — decision 3).
type CostEvent struct {
	Kind             string
	Model            string
	PromptVersion    string
	PromptTokens     int64
	CompletionTokens int64
	UsdMicros        int64
	// Estimated is cost_events.cost_source == 'estimated' (vs 'gateway').
	Estimated bool
	// WorkspaceID is attribution only — nullable, and never the account a
	// debit is applied to (credit_accounts and credit_entries key on
	// UserID: "每個帳號" is the user, migration 0060's own comment on why).
	WorkspaceID    pgtype.UUID
	UserID         pgtype.UUID // nullable: e.g. an anonymous catalogue search
	RefType        string      // "", RefCreationSession, RefRun or RefSkillVersion
	RefID          pgtype.UUID
	IdempotencyKey string
}

// DebitEntry is one credit_entries debit row, always pointing at the
// CostEvent it was computed from and always against a specific user's
// account (credit_entries.user_id is NOT NULL).
type DebitEntry struct {
	CostEventID string
	UserID      pgtype.UUID
	Credits     int64 // positive magnitude; the ledger's delta_credits is its negation
	// UsdMicros is the real cost this debit was computed from. Not
	// redundant with the cost event it points at: migration 0060's
	// credit_entries_debit_has_cost requires it on the row itself, so a
	// reconciliation can check the arithmetic of one entry without joining,
	// and so an entry survives its cost event being swept.
	UsdMicros      int64
	MarkupBps      int64 // the markup in effect when this entry was written (decision 2: never rewritten)
	Estimated      bool
	RefType        string
	RefID          pgtype.UUID
	IdempotencyKey string
}

// GrantEntry is one operator-initiated credit_entries row: grant, topup or
// adjustment (decision 10). MVP topup is the same mechanism as grant.
// OperatorID is not a ledger column — credit_entries carries no operator
// reference — it is only what the audit event records.
type GrantEntry struct {
	UserID         pgtype.UUID
	EntryKind      string // EntryGrant, EntryTopup or EntryAdjustment
	Credits        int64  // signed: positive for grant/topup, either sign for an adjustment
	Reason         string // SEC-011: required, becomes part of the audit trail
	OperatorID     pgtype.UUID
	IdempotencyKey string
}

// Store is the persistence port credit needs. Its shape mirrors
// db/queries/credit.sql and db/queries/cost.sql exactly — both already
// written and reviewed (migration 0060) — but no method here reuses one of
// those queries' own names: this codebase's query-ownership check (ADR-033,
// tools/devctl's query_owners.go) treats an interface method matching a
// registered query name as reserved, precisely to stop a consumer from
// hiding a *gen.Queries call behind an interface and evading ownership. A
// Postgres-backed Store (this package's one remaining follow-up, once
// `task gen:sql` has run for these already-written queries) is expected to
// be a thin, unsurprising wrapper: RecordCostEvent -> InsertCostEvent,
// ApplyDebit -> InsertCreditEntry + AdjustCreditBalance, and so on — see the
// per-method comments below for the exact query each maps to.
type Store interface {
	// Balance reads GetCreditBalance's balance_credits for a user. A user
	// with no credit_accounts row yet reads as 0 (EnsureCreditAccount has
	// not run for them, which is itself a fact worth the adapter logging,
	// not this interface's problem to expose).
	// Balance reads on tx when given, else on the pool; a gate inside a caller's
	// transaction must pass it or deadlock a one-connection pool.
	Balance(ctx context.Context, tx DBTX, userID pgtype.UUID) (int64, error)

	// RecordCostEvent maps to InsertCostEvent: one real platform spend,
	// inside tx. Idempotent on IdempotencyKey (the column's UNIQUE
	// constraint): existed=true on a replay means no new row was written
	// and id is the one already stored.
	RecordCostEvent(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error)

	// ApplyDebit maps to InsertCreditEntry followed by AdjustCreditBalance
	// (skipped on a 23505 replay, per credit.sql's own comment) in the same
	// tx as RecordCostEvent (decision 5). Idempotent on IdempotencyKey the
	// same way; existed=true means the balance was not touched again and
	// balanceAfter is simply the current balance.
	ApplyDebit(ctx context.Context, tx DBTX, d DebitEntry) (balanceAfter int64, existed bool, err error)

	// ApplyGrant maps to InsertCreditEntry (kind grant/topup/adjustment)
	// followed by AdjustCreditBalance, in tx.
	ApplyGrant(ctx context.Context, tx DBTX, g GrantEntry) (balanceAfter int64, err error)

	// RecentStatistics maps to GetLatestCostStatistics: the newest
	// cost_statistics row for kind, or ErrNoStatistics when none exists.
	RecentStatistics(ctx context.Context, kind string) (Statistics, error)

	// RecomputeStatistics maps to AggregateCostEventsWindow followed by
	// InsertCostStatistics for the window [windowStart, windowEnd) — one
	// atomic aggregate-then-persist, matching cost.sql's own comment that
	// this single pair serves both of decision 9's triggers ("Called both
	// event-driven (session end) and by the daily River job... same query,
	// different window bounds"): the caller decides when to call this and
	// with what bounds, not this interface.
	RecomputeStatistics(ctx context.Context, kind string, windowStart, windowEnd time.Time) (Statistics, error)

	// PurgeUser maps to PurgeUserCostEvents/PurgeUserCreditEntries, the two
	// user-scoped deletes account deletion needs — under the same "SET LOCAL
	// skillhub.purge = 'on'" gate the retention sweeps use.
	//
	// It is deliberately NOT the retention sweep: PurgeExpiredCostEvents and
	// PurgeExpiredCreditEntries filter on created_at alone and would leave a
	// deleted account's rows on file until they aged out. This comment claimed
	// otherwise until the adversarial review of 2026-09-08 read the SQL.
	//
	// Note this takes a userID, not a workspaceID: it cannot be assigned
	// directly to workspace/purge.go's WorkspacePurge (which purges per
	// workspace). See this batch's report for the composition-root
	// resolution that shape needs.
	PurgeUser(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) error
}
