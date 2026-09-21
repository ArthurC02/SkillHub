package wiring

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

func TestAnAbsentCreationQueueDoesNotLookConfigured(t *testing.T) {
	var client *river.Client[pgx.Tx]
	if queue := NewCreationQueue(client); queue != nil {
		t.Error("an unconfigured client became a creation queue")
	}
}

func TestCreationStepJobRoundTripsTheCommand(t *testing.T) {
	command := creation.JobArgs{
		SessionID:   pgtype.UUID{Bytes: [16]byte{0: 1}, Valid: true},
		WorkspaceID: pgtype.UUID{Bytes: [16]byte{0: 2}, Valid: true},
		Revision:    3,
		ReceiptID:   pgtype.UUID{Bytes: [16]byte{0: 4}, Valid: true},
	}
	if got := NewCreationStepArgs(command); got.Command() != command {
		t.Fatalf("command = %+v, want %+v", got.Command(), command)
	}
}

func TestCreditRecomputeJobRoundTripsTheCommand(t *testing.T) {
	command := credit.RecomputeArgs{StatKind: credit.CostKind("model"), WindowSeconds: 86_400}
	if got := NewCreditRecomputeArgs(command); got.Command() != command {
		t.Fatalf("command = %+v, want %+v", got.Command(), command)
	}
}
