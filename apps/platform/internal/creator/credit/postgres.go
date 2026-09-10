package credit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// uniqueViolation is Postgres' 23505. The codebase's idempotency convention
// is to catch it rather than upsert (credit.sql's own comment says so, citing
// registry.go and evidence/service.go), so both ledger writes below do that.
const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// Pool is the read handle PostgresStore needs for the two methods that take
// no caller transaction (Balance and the statistics reads). Declared here
// rather than importing pgxpool so a test can supply a single connection.
type Pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresStore is the [Store] this package was written against, on top of
// the sqlc output for db/queries/credit.sql and db/queries/cost.sql.
//
// Every method here is a thin translation and nothing more: the markup, the
// ceiling division, the -50 floor, the p95 threshold and its sample-count
// fallback all live in service.go and money.go, tested against a fake Store.
// This file exists so that logic reaches a real database, and it is the one
// remaining follow-up doc.go named.
//
// No method reuses a registered query's name. That is not cosmetic:
// tools/devctl's query-ownership check (ADR-033) treats an interface method
// matching a query name as evidence that someone is hiding a *gen.Queries
// call behind an interface to dodge ownership, so RecordCostEvent wraps
// InsertCostEvent, SweepExpiredRows wraps the two PurgeExpired* queries, and
// so on.
type PostgresStore struct {
	Pool Pool
}

// NewPostgresStore returns a Store backed by pool.
func NewPostgresStore(pool Pool) *PostgresStore { return &PostgresStore{Pool: pool} }

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) q(tx DBTX) *gen.Queries {
	if tx != nil {
		return gen.New(tx)
	}
	return gen.New(s.Pool)
}

// Balance reads the materialized balance. A user with no credit_accounts row
// reads as 0 rather than as an error: the row is created on first write
// (EnsureCreditAccount), so its absence means "has never spent or been
// granted anything", which is exactly a zero balance.
func (s *PostgresStore) Balance(ctx context.Context, tx DBTX, userID pgtype.UUID) (int64, error) {
	acct, err := s.q(tx).GetCreditBalance(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("credit: read balance: %w", err)
	}
	return acct.BalanceCredits, nil
}

// RecordCostEvent writes one real-spend row, idempotent on IdempotencyKey.
// On a replay it reads back the id the first call stored — Charge points the
// debit entry at it, and a debit that cannot name its cost event is the
// "扣了錢但答不出為什麼" ADR-068 decision 3 forbids.
func (s *PostgresStore) RecordCostEvent(ctx context.Context, tx DBTX, e CostEvent) (string, bool, error) {
	q := s.q(tx)
	source := "gateway"
	if e.Estimated {
		source = "estimated"
	}
	row, err := q.InsertCostEvent(ctx, gen.InsertCostEventParams{
		Kind:             e.Kind,
		Model:            e.Model,
		PromptVersion:    nullString(e.PromptVersion),
		PromptTokens:     e.PromptTokens,
		CompletionTokens: e.CompletionTokens,
		UsdMicros:        e.UsdMicros,
		CostSource:       source,
		WorkspaceID:      e.WorkspaceID,
		UserID:           e.UserID,
		RefType:          nullString(e.RefType),
		RefID:            e.RefID,
		IdempotencyKey:   e.IdempotencyKey,
	})
	if err == nil {
		return pgconv.UUIDString(row.ID), false, nil
	}
	if !isUniqueViolation(err) {
		return "", false, fmt.Errorf("credit: record cost event: %w", err)
	}
	existing, lookupErr := q.GetCostEventByIdempotencyKey(ctx, e.IdempotencyKey)
	if lookupErr != nil {
		return "", false, fmt.Errorf("credit: read replayed cost event: %w", lookupErr)
	}
	return pgconv.UUIDString(existing), true, nil
}

// ApplyDebit writes the debit entry and moves the balance in the same
// transaction. The delta is the negation of the positive magnitude the
// domain works in. On a replay the balance is deliberately NOT touched again
// and the current balance is returned instead — double-applying is the whole
// failure this key exists to prevent.
func (s *PostgresStore) ApplyDebit(ctx context.Context, tx DBTX, d DebitEntry) (int64, bool, error) {
	q := s.q(tx)
	if err := q.EnsureCreditAccount(ctx, d.UserID); err != nil {
		return 0, false, fmt.Errorf("credit: ensure account: %w", err)
	}
	markup := int32(d.MarkupBps)
	usd := d.UsdMicros
	_, err := q.InsertCreditEntry(ctx, gen.InsertCreditEntryParams{
		UserID:         d.UserID,
		Kind:           EntryDebit,
		DeltaCredits:   -d.Credits,
		UsdMicros:      &usd,
		MarkupBps:      &markup,
		RefType:        nullString(d.RefType),
		RefID:          d.RefID,
		CostEventID:    uuidFromString(d.CostEventID),
		Estimated:      d.Estimated,
		IdempotencyKey: d.IdempotencyKey,
	})
	if err != nil {
		if !isUniqueViolation(err) {
			return 0, false, fmt.Errorf("credit: insert debit: %w", err)
		}
		balance, readErr := s.balanceIn(ctx, q, d.UserID)
		if readErr != nil {
			return 0, false, readErr
		}
		return balance, true, nil
	}
	balance, err := q.AdjustCreditBalance(ctx, gen.AdjustCreditBalanceParams{
		DeltaCredits: -d.Credits,
		UserID:       d.UserID,
	})
	if err != nil {
		return 0, false, fmt.Errorf("credit: apply debit to balance: %w", err)
	}
	return balance, false, nil
}

// ApplyGrant is the operator's side of the ledger (decision 10). Signed:
// positive for grant and topup, either sign for a corrective adjustment.
// Idempotent on the same key convention as ApplyDebit, so a retried operator
// action tops the account up once.
func (s *PostgresStore) ApplyGrant(ctx context.Context, tx DBTX, g GrantEntry) (int64, error) {
	q := s.q(tx)
	if err := q.EnsureCreditAccount(ctx, g.UserID); err != nil {
		return 0, fmt.Errorf("credit: ensure account: %w", err)
	}
	_, err := q.InsertCreditEntry(ctx, gen.InsertCreditEntryParams{
		UserID:         g.UserID,
		Kind:           g.EntryKind,
		DeltaCredits:   g.Credits,
		Estimated:      false,
		IdempotencyKey: g.IdempotencyKey,
	})
	if err != nil {
		if !isUniqueViolation(err) {
			return 0, fmt.Errorf("credit: insert grant: %w", err)
		}
		return s.balanceIn(ctx, q, g.UserID)
	}
	balance, err := q.AdjustCreditBalance(ctx, gen.AdjustCreditBalanceParams{
		DeltaCredits: g.Credits,
		UserID:       g.UserID,
	})
	if err != nil {
		return 0, fmt.Errorf("credit: apply grant to balance: %w", err)
	}
	return balance, nil
}

func (s *PostgresStore) balanceIn(ctx context.Context, q *gen.Queries, userID pgtype.UUID) (int64, error) {
	acct, err := q.GetCreditBalance(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("credit: read balance: %w", err)
	}
	return acct.BalanceCredits, nil
}

// RecentStatistics reports the newest window for kind, or ErrNoStatistics —
// which gate ① reads as "fall back to the configured constant" rather than
// as a zero threshold that would let everything through.
func (s *PostgresStore) RecentStatistics(ctx context.Context, kind string) (Statistics, error) {
	row, err := s.q(nil).GetLatestCostStatistics(ctx, kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Statistics{}, ErrNoStatistics
	}
	if err != nil {
		return Statistics{}, fmt.Errorf("credit: read statistics: %w", err)
	}
	return Statistics{
		SampleCount:  int(row.SampleCount),
		P50UsdMicros: deref(row.P50UsdMicros),
		P90UsdMicros: deref(row.P90UsdMicros),
		P95UsdMicros: deref(row.P95UsdMicros),
		MaxUsdMicros: deref(row.MaxUsdMicros),
	}, nil
}

// RecomputeStatistics aggregates the window and persists the result, the
// single pair cost.sql says serves both of decision 9's triggers. The
// percentiles are computed in SQL — there is one definition of p95 in this
// system, and it is not in Go.
func (s *PostgresStore) RecomputeStatistics(ctx context.Context, kind string, windowStart, windowEnd time.Time) (Statistics, error) {
	q := s.q(nil)
	agg, err := q.AggregateCostEventsWindow(ctx, gen.AggregateCostEventsWindowParams{
		Kind:        kind,
		WindowStart: pgconv.Timestamptz(windowStart),
		WindowEnd:   pgconv.Timestamptz(windowEnd),
	})
	if err != nil {
		return Statistics{}, fmt.Errorf("credit: aggregate window: %w", err)
	}
	if _, err := q.InsertCostStatistics(ctx, gen.InsertCostStatisticsParams{
		Kind:         kind,
		WindowStart:  pgconv.Timestamptz(windowStart),
		WindowEnd:    pgconv.Timestamptz(windowEnd),
		SampleCount:  agg.SampleCount,
		P50UsdMicros: &agg.P50UsdMicros,
		P90UsdMicros: &agg.P90UsdMicros,
		P95UsdMicros: &agg.P95UsdMicros,
		MaxUsdMicros: &agg.MaxUsdMicros,
	}); err != nil {
		return Statistics{}, fmt.Errorf("credit: persist statistics: %w", err)
	}
	return Statistics{
		SampleCount:  int(agg.SampleCount),
		P50UsdMicros: agg.P50UsdMicros,
		P90UsdMicros: agg.P90UsdMicros,
		P95UsdMicros: agg.P95UsdMicros,
		MaxUsdMicros: agg.MaxUsdMicros,
	}, nil
}

// PurgeUser deletes everything one account's paid calls left behind, whatever
// its age. It takes the full pgx.Tx rather than DBTX because it must SET
// LOCAL the purge guard both tables' immutability triggers check.
func (s *PostgresStore) PurgeUser(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) error {
	if err := enablePurge(ctx, tx); err != nil {
		return err
	}
	q := gen.New(tx)
	// Entries first, and the order is load-bearing: credit_entries.cost_event_id
	// is a foreign key into cost_events, so deleting the events first fails on
	// credit_entries_cost_event_id_fkey. Found by the account-purge integration
	// test on the day this was wired, which is the only way it could have been —
	// nothing about the two calls looks ordered.
	if _, err := q.PurgeUserCreditEntries(ctx, userID); err != nil {
		return fmt.Errorf("credit: purge user credit entries: %w", err)
	}
	if _, err := q.PurgeUserCostEvents(ctx, userID); err != nil {
		return fmt.Errorf("credit: purge user cost events: %w", err)
	}
	return nil
}

// SweepExpiredRows is the retention half PurgeUser deliberately is not: both
// tables' time-based sweep, deleting everything written before cutoff. It is
// named for what it does rather than for either query it calls, per the
// naming rule in this file's type comment.
//
// It is not on [Store]: the interface is what the domain service needs, and
// the service never runs retention — cmd/maintenance does, on a schedule,
// against this concrete type.
func (s *PostgresStore) SweepExpiredRows(ctx context.Context, tx pgx.Tx, cutoff time.Time) (entries int64, events int64, err error) {
	if err := enablePurge(ctx, tx); err != nil {
		return 0, 0, err
	}
	q := gen.New(tx)
	at := pgconv.Timestamptz(cutoff)
	// Entries before events, for PurgeUser's foreign-key reason. A sweep that
	// removed an event still referenced by a younger entry would fail; one that
	// removed it successfully would leave a debit pointing at nothing.
	entries, err = q.PurgeExpiredCreditEntries(ctx, at)
	if err != nil {
		return 0, 0, fmt.Errorf("credit: sweep expired credit entries: %w", err)
	}
	events, err = q.PurgeExpiredCostEvents(ctx, at)
	if err != nil {
		return 0, 0, fmt.Errorf("credit: sweep expired cost events: %w", err)
	}
	return entries, events, nil
}

// enablePurge opens the door both tables' immutability triggers keep shut.
// Transaction-local, so it closes again on commit or rollback whatever the
// caller does next.
func enablePurge(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return fmt.Errorf("credit: enable purge: %w", err)
	}
	return nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func uuidFromString(s string) pgtype.UUID {
	var u pgtype.UUID
	if s == "" {
		return u
	}
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return u
}
