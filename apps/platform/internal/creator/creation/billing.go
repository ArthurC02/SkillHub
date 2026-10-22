package creation

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
