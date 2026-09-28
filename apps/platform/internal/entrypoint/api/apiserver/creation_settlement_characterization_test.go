package apiserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
)

type settledStepRow struct {
	kind, costSource, refType string
	usdMicros                 int64
	workspaceID, refID        pgtype.UUID
	debits                    int
}

func settledStep(t *testing.T, tx pgx.Tx, key string) settledStepRow {
	t.Helper()
	var row settledStepRow
	if err := tx.QueryRow(context.Background(), `
		SELECT kind, cost_source, ref_type, usd_micros, workspace_id, ref_id,
		       (SELECT count(*) FROM credit_entries WHERE idempotency_key = $1 AND kind = 'debit')
		FROM cost_events WHERE idempotency_key = $1`, key).Scan(
		&row.kind, &row.costSource, &row.refType, &row.usdMicros, &row.workspaceID, &row.refID, &row.debits); err != nil {
		t.Fatalf("reading the cost event keyed %q: %v", key, err)
	}
	return row
}

func TestASettledCreationStepIsBookedUnderItsSessionAndRevision(t *testing.T) {
	_, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-settle-oneconn")
	workspaceID := mustParseUUID(t, member.workspaceID)

	pool := creditsTestPool(t)
	svc, err := wiring.NewCreditService(pool)
	if err != nil {
		t.Fatal(err)
	}
	target := &creation.Service{}
	wiring.WireCreationCredit(target, svc, pool)

	measured := .003
	for _, tc := range []settlementCase{
		{"a step whose cost the gateway reported", &measured, 3000, "gateway", 1},
		{"a step whose cost is unknown books the reservation", nil, 100000, "estimated", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			sessionID := mustParseUUID(t, uuid.NewString())
			step := creation.StepSettlement{
				WorkspaceID: workspaceID, SessionID: sessionID, Revision: 7, CostUSD: tc.cost, ReservedUSD: .10,
			}
			if err := target.Billing.Settle(ctx, tx, step); err != nil {
				t.Fatalf("settling the step: %v", err)
			}

			got := settledStep(t, tx, fmt.Sprintf("creation:%s:7", uuid.UUID(sessionID.Bytes).String()))
			tc.assertBookedUnderTheSession(t, got, sessionID, workspaceID)
		})
	}
}

type settlementCase struct {
	name       string
	cost       *float64
	wantMicros int64
	wantSource string
	wantDebits int
}

func (tc settlementCase) assertBookedUnderTheSession(t *testing.T, got settledStepRow, sessionID, workspaceID pgtype.UUID) {
	t.Helper()
	if got.kind != "creation_step" || got.refType != "creation_session" || got.refID != sessionID {
		t.Errorf("booked as %s against %s %v, want creation_step against creation_session %v",
			got.kind, got.refType, got.refID, sessionID)
	}
	if got.workspaceID != workspaceID {
		t.Errorf("booked to workspace %v, want %v", got.workspaceID, workspaceID)
	}
	if got.usdMicros != tc.wantMicros || got.costSource != tc.wantSource {
		t.Errorf("usd_micros=%d cost_source=%s, want %d %s", got.usdMicros, got.costSource, tc.wantMicros, tc.wantSource)
	}
	if got.debits != tc.wantDebits {
		t.Errorf("debit entries = %d, want %d", got.debits, tc.wantDebits)
	}
}
