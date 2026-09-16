package credit

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestASessionSummaryAddsEveryStepAndBelongsToWhoeverTookTheLastOne(t *testing.T) {
	session := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	alice := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	bob := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	at := func(minute int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Date(2026, 9, 17, 12, minute, 0, 0, time.UTC), Valid: true}
	}
	for _, tc := range []struct {
		what  string
		steps []sessionStep
		want  sessionCostSummary
	}{
		{"one measured step", []sessionStep{{alice, 3000, CostSourceGateway, at(1)}},
			sessionCostSummary{session, alice, 3000, 1, false, at(1)}},
		{"an estimated step marks the whole session estimated", []sessionStep{
			{alice, 3000, CostSourceGateway, at(1)}, {alice, 5000, CostSourceEstimated, at(2)}, {alice, 1000, CostSourceGateway, at(3)},
		}, sessionCostSummary{session, alice, 9000, 3, true, at(3)}},
		{"the last step's user owns the session", []sessionStep{
			{alice, 1000, CostSourceGateway, at(1)}, {bob, 2000, CostSourceGateway, at(2)},
		}, sessionCostSummary{session, bob, 3000, 2, false, at(2)}},
	} {
		if got := summarizeSession(session, tc.steps); got != tc.want {
			t.Errorf("%s: summary = %+v, want %+v", tc.what, got, tc.want)
		}
	}
	if statisticsCostSource != CostSourceGateway {
		t.Errorf("statistics read %q costs, want only gateway-measured ones", statisticsCostSource)
	}
}
