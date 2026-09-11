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

const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

type Pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresStore struct {
	Pool Pool
}

func NewPostgresStore(pool Pool) *PostgresStore { return &PostgresStore{Pool: pool} }

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) q(tx DBTX) *gen.Queries {
	if tx != nil {
		return gen.New(tx)
	}
	return gen.New(s.Pool)
}

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
		WindowEnd:    row.WindowEnd.Time,
	}, nil
}

func (s *PostgresStore) RecomputeStatistics(ctx context.Context, kind string, windowStart, windowEnd time.Time) (Statistics, error) {
	q := s.q(nil)
	var agg gen.AggregateCostEventsWindowRow
	var err error
	if kind == KindCreationSession {
		var row gen.AggregateSessionSummariesWindowRow
		row, err = q.AggregateSessionSummariesWindow(ctx, gen.AggregateSessionSummariesWindowParams{
			WindowStart: pgconv.Timestamptz(windowStart),
			WindowEnd:   pgconv.Timestamptz(windowEnd),
		})
		agg = gen.AggregateCostEventsWindowRow(row)
	} else {
		agg, err = q.AggregateCostEventsWindow(ctx, gen.AggregateCostEventsWindowParams{
			Kind:        kind,
			WindowStart: pgconv.Timestamptz(windowStart),
			WindowEnd:   pgconv.Timestamptz(windowEnd),
		})
	}
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
		WindowEnd:    windowEnd,
	}, nil
}

func (s *PostgresStore) PurgeUser(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) error {
	if err := enablePurge(ctx, tx); err != nil {
		return err
	}
	q := gen.New(tx)

	// Entries before events: credit_entries.cost_event_id is a foreign key
	// into cost_events, so the reverse order fails on the constraint.
	if _, err := q.PurgeUserCreditEntries(ctx, userID); err != nil {
		return fmt.Errorf("credit: purge user credit entries: %w", err)
	}
	if _, err := q.PurgeUserCostEvents(ctx, userID); err != nil {
		return fmt.Errorf("credit: purge user cost events: %w", err)
	}
	if _, err := q.PurgeUserSessionCostSummaries(ctx, userID); err != nil {
		return fmt.Errorf("credit: purge user session summaries: %w", err)
	}
	return nil
}

func (s *PostgresStore) SummarizeSession(ctx context.Context, tx DBTX, sessionID pgtype.UUID) error {
	return s.q(tx).UpsertSessionCostSummary(ctx, sessionID)
}

func (s *PostgresStore) SweepSessionSummaries(ctx context.Context, windowStart, idleBefore time.Time) (int64, error) {
	return s.q(nil).SweepSessionCostSummaries(ctx, gen.SweepSessionCostSummariesParams{
		WindowStart: pgconv.Timestamptz(windowStart),
		IdleBefore:  pgconv.Timestamptz(idleBefore),
	})
}

func (s *PostgresStore) SweepExpiredRows(ctx context.Context, tx pgx.Tx, cutoff time.Time) (entries int64, events int64, err error) {
	if err := enablePurge(ctx, tx); err != nil {
		return 0, 0, err
	}
	q := gen.New(tx)
	at := pgconv.Timestamptz(cutoff)

	// Entries before events, for the same foreign-key reason as PurgeUser.
	entries, err = q.PurgeExpiredCreditEntries(ctx, at)
	if err != nil {
		return 0, 0, fmt.Errorf("credit: sweep expired credit entries: %w", err)
	}
	events, err = q.PurgeExpiredCostEvents(ctx, at)
	if err != nil {
		return 0, 0, fmt.Errorf("credit: sweep expired cost events: %w", err)
	}
	if _, err := q.PurgeExpiredSessionCostSummaries(ctx, at); err != nil {
		return 0, 0, fmt.Errorf("credit: sweep expired session summaries: %w", err)
	}
	return entries, events, nil
}

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
