package creation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StepSettlement struct {
	WorkspaceID pgtype.UUID
	SessionID   pgtype.UUID
	Revision    int64
	CostUSD     *float64
	ReservedUSD float64
}

type BillingHooks struct {
	ModelCallsHalted func(ctx context.Context) bool
	CanStartFunc     func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)
	ReserveFunc      func(ctx context.Context, workspaceID pgtype.UUID, reservedUSD float64) (bool, error)
	SettleFunc       func(ctx context.Context, tx pgx.Tx, step StepSettlement) error
	SessionEndedFunc func(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error
	SettleDeadline   time.Duration
}

func (h BillingHooks) settleDeadline() time.Duration {
	if h.SettleDeadline > 0 {
		return h.SettleDeadline
	}
	return settleTimeout
}

func (h BillingHooks) CanStart(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
	if h.CanStartFunc == nil {
		return true, nil
	}
	return h.CanStartFunc(ctx, workspaceID)
}

func (h BillingHooks) Reserve(ctx context.Context, workspaceID pgtype.UUID, reservedUSD float64) (bool, error) {
	if h.ReserveFunc == nil {
		return true, nil
	}
	return h.ReserveFunc(ctx, workspaceID, reservedUSD)
}

func (h BillingHooks) Settle(ctx context.Context, tx pgx.Tx, step StepSettlement) error {
	if h.SettleFunc == nil {
		return nil
	}
	return h.SettleFunc(ctx, tx, step)
}

func (h BillingHooks) SessionEnded(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error {
	if h.SessionEndedFunc == nil {
		return nil
	}
	return h.SessionEndedFunc(ctx, tx, sessionID)
}

const costObservationWindow = 24 * time.Hour

const (
	costCoverageKnown     = "known"
	costCoverageUnsettled = "unsettled"
	costCoverageUnknown   = "unknown"
)

type costObservation struct {
	known, unsettled, unknown int64
	usd, maxUSD               float64
}

func PublishCostGauges(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		until := time.Now().UTC()
		rows, err := gen.New(pool).ListCreationCostObservations(ctx, gen.ListCreationCostObservationsParams{
			Since: pgconv.Timestamptz(until.Add(-costObservationWindow)), Until: pgconv.Timestamptz(until),
		})
		if err != nil {
			withdrawCostObservation()
			return err
		}
		stats, err := observeCreationCosts(rows)
		if err != nil {
			withdrawCostObservation()
			return err
		}
		publishCostObservation(stats, until)
		return nil
	}
}

func observeCreationCosts(rows []gen.ListCreationCostObservationsRow) (costObservation, error) {
	var stats costObservation
	for _, row := range rows {
		coverage, cost := creationCostCoverage(row)
		switch coverage {
		case costCoverageKnown:
			stats.known++
			stats.usd += cost
			stats.maxUSD = max(stats.maxUSD, cost)
		case costCoverageUnsettled:
			stats.unsettled++
		case costCoverageUnknown:
			stats.unknown++
		}
	}
	if !finite(stats.usd) {
		return costObservation{}, errors.New("creation cost observation overflow")
	}
	return stats, nil
}

func creationCostCoverage(row gen.ListCreationCostObservationsRow) (string, float64) {
	var unknown *bool
	if err := json.Unmarshal(row.UsageUnknown, &unknown); err != nil || unknown == nil || *unknown {
		return costCoverageUnknown, 0
	}
	spent, knownSpent := observedCostNumber(row.SpentUsd)
	reserved, knownReserved := observedCostNumber(row.ReservedUsd)
	if !knownSpent || !knownReserved {
		return costCoverageUnknown, 0
	}
	if reserved > 0 {
		return costCoverageUnsettled, 0
	}
	return costCoverageKnown, spent
}

func observedCostNumber(raw json.RawMessage) (float64, bool) {
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	if value > 0 && value < math.MaxFloat64 {
		return value, true
	}
	exact, ok := new(big.Rat).SetString(string(raw))
	if !ok || exact.Sign() < 0 || (value == 0 && exact.Sign() != 0) || exact.Cmp(new(big.Rat).SetFloat64(math.MaxFloat64)) > 0 {
		return 0, false
	}
	return value, true
}

func withdrawCostObservation() {
	metrics.CreationCostSessions.Reset()
	metrics.CreationKnownSpend.Reset()
	metrics.CreationMaxKnownCost.Reset()
	metrics.CreationCostObservedAt.Reset()
	metrics.CreationCostObservationSuccess.WithLabelValues().Set(0)
}

func publishCostObservation(stats costObservation, until time.Time) {
	metrics.CreationCostSessions.WithLabelValues(costCoverageKnown).Set(float64(stats.known))
	metrics.CreationCostSessions.WithLabelValues(costCoverageUnsettled).Set(float64(stats.unsettled))
	metrics.CreationCostSessions.WithLabelValues(costCoverageUnknown).Set(float64(stats.unknown))
	if stats.known == 0 {
		metrics.CreationKnownSpend.Reset()
		metrics.CreationMaxKnownCost.Reset()
	} else {
		metrics.CreationKnownSpend.WithLabelValues().Set(stats.usd)
		metrics.CreationMaxKnownCost.WithLabelValues().Set(stats.maxUSD)
	}
	metrics.CreationCostObservedAt.WithLabelValues().Set(float64(until.Unix()))
	metrics.CreationCostObservationSuccess.WithLabelValues().Set(1)
}
