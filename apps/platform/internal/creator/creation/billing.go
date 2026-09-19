package creation

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type CreationBilling interface {
	CanStart(ctx context.Context, workspaceID pgtype.UUID) (bool, error)
	Reserve(ctx context.Context, workspaceID pgtype.UUID, reservedUSD float64) (bool, error)
	Settle(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, costUSD *float64, reservedUSD float64) error
	SessionEnded(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error
}

type BillingHooks struct {
	CanStartFunc     func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)
	ReserveFunc      func(ctx context.Context, workspaceID pgtype.UUID, reservedUSD float64) (bool, error)
	SettleFunc       func(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, costUSD *float64, reservedUSD float64) error
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

func (h BillingHooks) Settle(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, costUSD *float64, reservedUSD float64) error {
	if h.SettleFunc == nil {
		return nil
	}
	return h.SettleFunc(ctx, tx, workspaceID, sessionID, revision, costUSD, reservedUSD)
}

func (h BillingHooks) SessionEnded(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error {
	if h.SessionEndedFunc == nil {
		return nil
	}
	return h.SessionEndedFunc(ctx, tx, sessionID)
}
