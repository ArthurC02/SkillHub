package apiserver_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type runCreditLedger struct {
	settle func(context.Context, pgx.Tx, run.RunSettlement) error
}

func (runCreditLedger) CreditsForUSD(float64) (int64, bool) { return 0, false }
func (runCreditLedger) Reserve(context.Context, pgx.Tx, pgtype.UUID, float64) (bool, error) {
	return true, nil
}
func (l runCreditLedger) Settle(ctx context.Context, tx pgx.Tx, r run.RunSettlement) error {
	return l.settle(ctx, tx, r)
}
func (runCreditLedger) FinalCostRecorded(context.Context, pgtype.UUID) (bool, error) {
	return false, nil
}
