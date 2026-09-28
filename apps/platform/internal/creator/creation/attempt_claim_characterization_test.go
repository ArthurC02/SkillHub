package creation

import (
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOnlyALiveQueuedSessionHoldingThisReceiptAwaitsTheAttempt(t *testing.T) {
	receipt := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	future := pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}
	past := pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}
	a := JobArgs{ReceiptID: receipt}
	cases := []struct {
		name   string
		state  State
		active pgtype.UUID
		expiry pgtype.Timestamptz
		want   bool
	}{
		{"queued, live, holding this receipt", StateQueued, receipt, future, true},
		{"already working", StateWorking, receipt, future, false},
		{"holding another receipt", StateQueued, other, future, false},
		{"expired", StateQueued, receipt, past, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := gen.CreationSession{State: string(c.state), ExpiresAt: c.expiry}
			if got := sessionAwaitsAttempt(row, envelope{ActiveReceipt: c.active}, a); got != c.want {
				t.Fatalf("sessionAwaitsAttempt = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOnlyAQueuedReceiptAtTheExpectedRevisionAwaitsTheAttempt(t *testing.T) {
	a := JobArgs{Revision: 3}
	cases := []struct {
		name     string
		status   string
		revision int64
		want     bool
	}{
		{"queued at the expected revision", "queued", 3, true},
		{"already running", "running", 3, false},
		{"queued at another revision", "queued", 4, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := gen.CreationReceipt{Status: c.status, ExpectedRevision: c.revision}
			if got := receiptAwaitsAttempt(r, a); got != c.want {
				t.Fatalf("receiptAwaitsAttempt = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOnlyTheRunningReceiptTheWorkingSessionStillHoldsConcludesTheAttempt(t *testing.T) {
	receipt := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	a := JobArgs{ReceiptID: receipt}
	cases := []struct {
		name   string
		state  State
		active pgtype.UUID
		status string
		want   bool
	}{
		{"working, holding it, running", StateWorking, receipt, "running", true},
		{"no longer working", StateCancelled, receipt, "running", false},
		{"holding another receipt", StateWorking, other, "running", false},
		{"recovered as unknown", StateWorking, receipt, "unknown", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := attemptStillCurrent(c.state, envelope{ActiveReceipt: c.active}, a, gen.CreationReceipt{Status: c.status})
			if got != c.want {
				t.Fatalf("attemptStillCurrent = %v, want %v", got, c.want)
			}
		})
	}
}
