package creation

import (
	"context"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"math"
	"strings"
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

func TestDiagramInterpretationRequiresAllSectionsBeforeSaving(t *testing.T) {
	valid := `{"nodes":["start"],"conditions":[],"branches":[],"uncertainties":[]}`
	for _, value := range []string{"legacy paragraph", `{"nodes":["start"]}`, `{"nodes":[],"conditions":[],"branches":[],"uncertainties":[]}`, `{"nodes":["start"],"conditions":[],"branches":[],"uncertainties":[" "]}`} {
		p := Snapshot{Brief: "task", BriefConfirmed: true, DiagramFingerprint: "digest", DiagramConfirmed: true, DiagramUnderstanding: value}
		if validDiagramInterpretation(value) || confirmed(p) {
			t.Errorf("invalid interpretation accepted: %s", value)
		}
		p.DiagramFingerprint = ""
		if confirmed(p) {
			t.Errorf("interpretation without an uploaded image bypassed confirmation: %s", value)
		}
	}
	if !validDiagramInterpretation(valid) {
		t.Fatal("complete sections rejected")
	}
}
func TestValidateToolKeepsNewDraftAndRequiresConfirmation(t *testing.T) {
	for _, confirmedBrief := range []bool{false, true} {
		calls := 0
		s := Service{ValidateDraft: func(_ context.Context, draft llmclient.GeneratedSkill) (string, string, bool, error) {
			calls++
			if draft.Body != "newly proposed body" {
				t.Error("validated stale content")
			}
			return "new-hash", "actual finding", true, nil
		}}
		e := envelope{Snapshot: Snapshot{Brief: "task", BriefConfirmed: confirmedBrief}, Limits: testLimits()}
		state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, &llmclient.CreationStepResponse{
			Outcome: "tool_intent", Message: "validate", Brief: "task", ToolIntent: &llmclient.CreationToolIntent{Kind: "validate_draft"},
			Draft: &llmclient.GeneratedSkill{Body: "newly proposed body"},
		})
		if !confirmedBrief {
			if err == nil || calls != 0 {
				t.Fatal("tool bypassed confirmation")
			}
			continue
		}
		if err != nil || !next || state != "queued" || calls != 1 || e.Snapshot.Draft == nil || e.Snapshot.Draft.Validation != "actual finding" || !e.Snapshot.Draft.Blocked {
			t.Fatalf("new tool draft lost: state=%s next=%t err=%v snapshot=%+v", state, next, err, e.Snapshot)
		}
	}
}

// A step appends up to two messages (assistant + tool); a snapshot already at
// 97 messages would be paid for and then refused by proposal()'s MaxMessages
// gate, so canSpend must refuse it up front.
func TestCanSpendRefusesNearMessageCeiling(t *testing.T) {
	p := Snapshot{Messages: make([]llmclient.CreationMessage, 97), BudgetUSD: 1}
	if canSpend(p, testLimits()) {
		t.Fatal("canSpend allowed a step that proposal() would refuse for message count")
	}
	p.Messages = make([]llmclient.CreationMessage, 96)
	if !canSpend(p, testLimits()) {
		t.Fatal("canSpend wrongly refused a snapshot with room for one more step")
	}
}

func TestDraftValidationReportTruncatedWithinLimit(t *testing.T) {
	report := []rune(strings.Repeat("x", MaxTextRunes+5000))
	marker := []rune("\n[findings truncated]")
	if len(report) > MaxTextRunes {
		report = append(report[:MaxTextRunes-len(marker)], marker...)
	}
	if len(report) > MaxTextRunes {
		t.Fatalf("truncated report still exceeds MaxTextRunes: %d", len(report))
	}
	if !strings.HasSuffix(string(report), "[findings truncated]") {
		t.Fatal("truncated report lost its marker")
	}
}

func TestAllowedToolsEmptyAtToolCallCeiling(t *testing.T) {
	if got := allowedTools(3, 3); len(got) != 0 {
		t.Fatalf("expected no tools once the budget is spent, got %v", got)
	}
	if got := allowedTools(2, 3); len(got) != 2 {
		t.Fatalf("expected both tools while budget remains, got %v", got)
	}
}

// The 5s headroom Go reserves for its own cleanup after a call must come out
// of the remaining time actually left on the deadline, not out of the full
// CallTimeout — otherwise a slow step before the model call (ResolveReference
// here) lets Go ask Python for more time than the deadline actually has.
func TestCallTimeoutSecondsAccountsForElapsedTime(t *testing.T) {
	callTimeout := 5 * time.Second
	callDeadline := time.Now().Add(callTimeout + 5*time.Second)
	time.Sleep(2 * time.Second)
	remaining, err := callTimeoutSeconds(callDeadline)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if remaining >= int(callTimeout.Seconds()) {
		t.Fatalf("elapsed time before the call was not deducted: remaining=%d callTimeout=%d", remaining, int(callTimeout.Seconds()))
	}
}

func TestCallTimeoutSecondsFailsClosedWhenDeadlineNearlyPassed(t *testing.T) {
	if _, err := callTimeoutSeconds(time.Now().Add(3 * time.Second)); err == nil {
		t.Fatal("expected an error when too little time remains for headroom plus a call")
	}
}

// TestReasonSentenceReplacesMessage: Go, not Python, owns the wording for a
// guard-rail reason code (05 R-46 (c)); an unrecognized code is refused
// rather than surfaced verbatim.
func TestReasonSentenceReplacesMessage(t *testing.T) {
	sentence, err := reasonSentence("confirm_brief_first")
	if err != nil || sentence != reasonSentences["confirm_brief_first"] {
		t.Fatalf("known reason not resolved: %q %v", sentence, err)
	}
	if _, err := reasonSentence("not_a_real_reason"); err == nil {
		t.Fatal("unknown reason code accepted")
	}
}

func TestCriteriaValidationRejectsTooManyOrTooLong(t *testing.T) {
	if err := validateCriteria(nil); err != nil {
		t.Fatalf("nil criteria should be valid: %v", err)
	}
	tooMany := make([]string, MaxAcceptanceCriteria+1)
	for i := range tooMany {
		tooMany[i] = "输出含 invoice_id 欄"
	}
	if err := validateCriteria(tooMany); err == nil {
		t.Fatal("13 criteria accepted")
	}
	if err := validateCriteria([]string{strings.Repeat("x", MaxCriterionRunes+1)}); err == nil {
		t.Fatal("501-rune criterion accepted")
	}
	if err := validateCriteria([]string{"  "}); err == nil {
		t.Fatal("blank criterion accepted")
	}
}
