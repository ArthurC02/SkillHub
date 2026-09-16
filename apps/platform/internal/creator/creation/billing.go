package creation

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CreationBilling interface {
	CanStart(context.Context, pgtype.UUID) (bool, error)
	Reserve(context.Context, pgtype.UUID, int64) (bool, error)
	Settle(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID, int64, *int64, int64) error
	SessionEnded(context.Context, pgx.Tx, pgtype.UUID) error
}

type BillingHooks struct {
	CanStartFunc     func(context.Context, pgtype.UUID) (bool, error)
	ReserveFunc      func(context.Context, pgtype.UUID, int64) (bool, error)
	SettleFunc       func(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID, int64, *int64, int64) error
	SessionEndedFunc func(context.Context, pgx.Tx, pgtype.UUID) error
}

func (h BillingHooks) CanStart(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
	if h.CanStartFunc == nil {
		return true, nil
	}
	return h.CanStartFunc(ctx, workspaceID)
}

func (h BillingHooks) Reserve(ctx context.Context, workspaceID pgtype.UUID, reservedUSDMicros int64) (bool, error) {
	if h.ReserveFunc == nil {
		return true, nil
	}
	return h.ReserveFunc(ctx, workspaceID, reservedUSDMicros)
}

func (h BillingHooks) Settle(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, usdMicros *int64, reservedUSDMicros int64) error {
	if h.SettleFunc == nil {
		return nil
	}
	return h.SettleFunc(ctx, tx, workspaceID, sessionID, revision, usdMicros, reservedUSDMicros)
}

func (h BillingHooks) SessionEnded(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error {
	if h.SessionEndedFunc == nil {
		return nil
	}
	return h.SessionEndedFunc(ctx, tx, sessionID)
}
