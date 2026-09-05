package creation

import (
	"context"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"math"
	"testing"
	"time"
)

func testLimits() Limits {
	return Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 8, MaxToolCalls: 3, CallTimeout: time.Second, SessionTimeout: time.Minute, Retention: time.Hour, MaxOutputTokens: 500}
}
func TestUnknownUsageRetainsReservation(t *testing.T) {
	zero := 0.0
	p := Snapshot{BudgetUSD: .1, SpentUSD: &zero, ReservedUSD: .1}
	settleCost(&p, .1, nil)
	if p.ReservedUSD != .1 || !p.UsageUnknown || canSpend(p, testLimits()) {
		t.Fatalf("unknown cost released budget: %+v", p)
	}
}
func TestKnownUsageSettlesOnlyActualSpend(t *testing.T) {
	zero, cost := 0.0, .03
	p := Snapshot{BudgetUSD: .2, SpentUSD: &zero, ReservedUSD: .1}
	settleCost(&p, .1, &llmclient.GatewayUsage{CostUSD: &cost})
	if p.ReservedUSD != 0 || *p.SpentUSD != cost || p.UsageUnknown {
		t.Fatalf("wrong settlement: %+v", p)
	}
}
func TestInvalidCostsNeverBecomeCredit(t *testing.T) {
	for _, cost := range []float64{-1, math.Inf(1), math.NaN()} {
		p := Snapshot{ReservedUSD: .1}
		settleCost(&p, .1, &llmclient.GatewayUsage{CostUSD: &cost})
		if p.ReservedUSD != .1 || !p.UsageUnknown {
			t.Fatal("invalid cost became credit")
		}
	}
}
func TestChangedConfirmedBriefCannotProduceDraft(t *testing.T) {
	calls := 0
	s := Service{ValidateDraft: func(context.Context, llmclient.GeneratedSkill) (string, string, bool, error) {
		calls++
		return "hash", "ok", false, nil
	}}
	e := envelope{Snapshot: Snapshot{Brief: "confirmed", BriefConfirmed: true}, Limits: testLimits()}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, &llmclient.CreationStepResponse{Outcome: "draft", Message: "proposal", Brief: "different", Draft: &llmclient.GeneratedSkill{Body: "body"}})
	if err != nil || next || state != "waiting_confirmation" || e.Snapshot.BriefConfirmed || e.Snapshot.Draft != nil || calls != 0 {
		t.Fatalf("confirmation bypass: %s %+v", state, e)
	}
}
func TestUnavailableReferenceBlocksDraft(t *testing.T) {
	e := envelope{Snapshot: Snapshot{Brief: "task", BriefConfirmed: true, References: []Reference{{Confirmed: true, Available: false}}}}
	s := Service{}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, &llmclient.CreationStepResponse{Outcome: "draft", Message: "draft", Draft: &llmclient.GeneratedSkill{}})
	if err == nil {
		t.Fatal("unavailable reference accepted")
	}
}
func TestLimitsFailClosed(t *testing.T) {
	l := testLimits()
	if !l.Valid() {
		t.Fatal("fixture invalid")
	}
	l.MaxCallCostUSD = math.Inf(1)
	if l.Valid() {
		t.Fatal("infinite budget")
	}
	t.Setenv("CREATION_LIMITS_JSON", "{}")
	if _, err := LimitsFromEnv(); err == nil {
		t.Fatal("missing limits enabled")
	}
}
