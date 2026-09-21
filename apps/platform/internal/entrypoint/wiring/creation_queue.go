package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

type CreationStepArgs struct {
	SessionID   pgtype.UUID `json:"session_id"`
	WorkspaceID pgtype.UUID `json:"workspace_id"`
	Revision    int64       `json:"expected_revision"`
	ReceiptID   pgtype.UUID `json:"receipt_id"`
}

func NewCreationStepArgs(a creation.JobArgs) CreationStepArgs {
	return CreationStepArgs{SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Revision: a.Revision, ReceiptID: a.ReceiptID}
}

func (a CreationStepArgs) Command() creation.JobArgs {
	return creation.JobArgs{
		SessionID:   a.SessionID,
		WorkspaceID: a.WorkspaceID,
		Revision:    a.Revision,
		ReceiptID:   a.ReceiptID,
	}
}

func (CreationStepArgs) Kind() string { return "creation_step" }

type CreationExpiryArgs struct{}

func (CreationExpiryArgs) Kind() string { return "creation_expiry" }

func NewCreationQueue(client *river.Client[pgx.Tx]) func(context.Context, pgx.Tx, creation.JobArgs) error {
	if client == nil {
		return nil
	}
	return func(ctx context.Context, tx pgx.Tx, command creation.JobArgs) error {
		_, err := client.InsertTx(ctx, tx, NewCreationStepArgs(command), &river.InsertOpts{MaxAttempts: 1})
		return err
	}
}
