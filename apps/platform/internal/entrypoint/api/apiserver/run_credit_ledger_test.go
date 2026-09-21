package apiserver_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type runCreditLedger struct {
	settle func(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID, *float64, float64) error
}

func (runCreditLedger) CreditsForUSD(float64) (int64, bool) { return 0, false }
func (runCreditLedger) Reserve(context.Context, pgx.Tx, pgtype.UUID, float64) (bool, error) {
	return true, nil
}
func (l runCreditLedger) Settle(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, cost *float64, reserved float64) error {
	return l.settle(ctx, tx, workspaceID, runID, cost, reserved)
}
func (runCreditLedger) FinalCostRecorded(context.Context, pgtype.UUID) (bool, error) {
	return false, nil
}
