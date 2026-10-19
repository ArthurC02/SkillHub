package wiring

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

func TestTheCreationStartGateLearnsTheFirstStepReserveFromTheSessionLimits(t *testing.T) {
	svc := &credit.Service{}
	WireCreationCredit(&creation.Service{Limits: creation.Limits{MaxCallCostUSD: 0.1}}, svc, nil)

	if got := svc.FirstStepReserveMicros[credit.KindCreationSession]; got != 100_000 {
		t.Fatalf("first creation step reserve = %d micros, want 100000 (US$0.10)", got)
	}
}
