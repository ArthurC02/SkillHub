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
	if got := allowedTools(3, 3, false, false); len(got) != 0 {
		t.Fatalf("expected no tools once the budget is spent, got %v", got)
	}
	if got := allowedTools(2, 3, false, false); len(got) != 2 {
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

// The reason code is the only thing Python is trusted to say about a refusal;
// the sentence the person reads comes from Go's table (05 R-46 (c)). Deleting
// the lookup in proposal() would hand the English fallback to a zh-TW reader.
func TestProposalReplacesTheMessageFromTheReasonTable(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, &llmclient.CreationStepResponse{Outcome: "clarification", Message: "tool unavailable", Reason: "tool_unavailable"})
	if err != nil || next || state != "waiting_input" {
		t.Fatalf("clarification with a reason: state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || last.Content != reasonSentences["tool_unavailable"] {
		t.Fatalf("Go did not own the sentence: %+v", last)
	}
	if _, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, &llmclient.CreationStepResponse{Outcome: "clarification", Message: "x", Reason: "made_up"}); err == nil {
		t.Fatal("an unknown reason code was accepted")
	}
}

func testLimitsForProposal() Limits {
	return Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 8, MaxToolCalls: 3, CallTimeout: 2 * time.Second, SessionTimeout: time.Minute, Retention: time.Hour, MaxOutputTokens: 1000}
}

// A confirmed brief re-proposed unchanged must not become a confirmation loop
// (2026-09-06 measurement: 3/15 sessions burned their step budget this way).
func TestProposalKeepsAConfirmedBriefWhenTheModelMerelyRestatesIt(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "整理輸入資料，依指定格式輸出摘要。", BriefConfirmed: true, AcceptanceCriteria: []string{"輸出含摘要"}}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, &llmclient.CreationStepResponse{Outcome: "confirm_brief", Message: "請再確認一次。", Brief: e.Snapshot.Brief, AcceptanceCriteria: []string{"輸出含摘要"}})
	if err != nil || next || state != "waiting_input" {
		t.Fatalf("restated brief: state=%q next=%v err=%v", state, next, err)
	}
	if !e.Snapshot.BriefConfirmed || e.Snapshot.PendingAction != "" {
		t.Fatalf("the confirmation was thrown away: %+v", e.Snapshot)
	}
}

// A model that says "draft" and hands over nothing gets one paid retry, then
// the turn goes back to the person (2026-09-06 run c).
func TestProposalRetriesOnceWhenTheDraftIsMissing(t *testing.T) {
	s := &Service{}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "clarification", Message: "draft missing", Reason: "draft_missing"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || !next || state != "queued" || e.Snapshot.DraftRetries != 1 {
		t.Fatalf("first draft_missing should requeue once: state=%q next=%v retries=%d err=%v", state, next, e.Snapshot.DraftRetries, err)
	}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || next || state != "waiting_input" || e.Snapshot.DraftRetries != 1 {
		t.Fatalf("second draft_missing should hand the turn back: state=%q next=%v retries=%d err=%v", state, next, e.Snapshot.DraftRetries, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Content != reasonSentences["draft_missing"] {
		t.Fatalf("the person did not get Go's sentence: %+v", last)
	}
}

// A proposal that changes the example input un-confirms the brief exactly as a
// changed criterion does: the Test Case prompt is confirmed input (run d).
func TestProposalTreatsAChangedSampleInputAsAChangedBrief(t *testing.T) {
	s := &Service{}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", AcceptanceCriteria: []string{"c"}, SampleInput: "old", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "confirm_brief", Message: "again", Brief: "b", AcceptanceCriteria: []string{"c"}, SampleInput: "new"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != "waiting_confirmation" || e.Snapshot.BriefConfirmed || e.Snapshot.SampleInput != "new" || e.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("changed sample_input did not reopen the confirmation: state=%q next=%v snap=%+v err=%v", state, next, e.Snapshot, err)
	}
	r.SampleInput = ""
	e.Snapshot.BriefConfirmed = true
	state, _, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || state != "waiting_input" || !e.Snapshot.BriefConfirmed {
		t.Fatalf("an empty sample_input must mean unchanged: state=%q confirmed=%v err=%v", state, e.Snapshot.BriefConfirmed, err)
	}
}

// After a run that was not met, a draft with the same hash as the one that
// ran is handed back to the model with a reason, at most MaxNudges times;
// then it is stored and the person is told (run g, 2026-09-06).
func TestProposalNudgesAnUnchangedDraftAfterAnUnmetRun(t *testing.T) {
	s := &Service{ValidateDraft: func(context.Context, llmclient.GeneratedSkill) (string, string, bool, error) {
		return "same-hash", "{}", false, nil
	}}
	zero := 0.0
	ran := &Draft{Revision: 3, ContentHash: "same-hash"}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero, Draft: ran, RunUnmet: true}}
	r := &llmclient.CreationStepResponse{Outcome: "draft", Message: "我已修正草稿。", Brief: "b", Draft: &llmclient.GeneratedSkill{Name: "x", Body: "same"}}
	for i := 1; i <= MaxNudges; i++ {
		state, next, err := s.proposal(context.Background(), identity.Workspace{}, int64(3+i), &e, r)
		if err != nil || !next || state != "queued" || e.Snapshot.Nudges != i || e.Snapshot.Draft != ran {
			t.Fatalf("nudge %d: state=%q next=%v nudges=%d err=%v", i, state, next, e.Snapshot.Nudges, err)
		}
		if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "逐位元相同") {
			t.Fatalf("nudge %d did not tell the model why: %+v", i, last)
		}
	}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || next || state != "draft_ready" || e.Snapshot.Nudges != MaxNudges {
		t.Fatalf("after MaxNudges the draft must be stored: state=%q next=%v nudges=%d err=%v", state, next, e.Snapshot.Nudges, err)
	}
	if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "assistant" || !strings.Contains(last.Content, "兩次都交回") {
		t.Fatalf("the person was not told: %+v", last)
	}
	// A draft with new content ends the nudging and clears the run flag.
	s.ValidateDraft = func(context.Context, llmclient.GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}
	e.Snapshot.Nudges = 0
	state, _, err = s.proposal(context.Background(), identity.Workspace{}, 10, &e, r)
	if err != nil || state != "draft_ready" || e.Snapshot.RunUnmet || e.Snapshot.Nudges != 0 {
		t.Fatalf("a changed draft is progress: state=%q unmet=%v nudges=%d err=%v", state, e.Snapshot.RunUnmet, e.Snapshot.Nudges, err)
	}
}

// A draft whose body does not walk every confirmed diagram node is handed
// back with the missing names; one that does is stored.
func TestProposalNudgesADraftThatSkipsDiagramNodes(t *testing.T) {
	s := &Service{ValidateDraft: func(_ context.Context, d llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h-" + d.Body, "{}", false, nil
	}}
	zero := 0.0
	understanding := `{"nodes":["收到報帳申請","送經理簽核","寄出付款通知"],"conditions":[],"branches":[],"uncertainties":[]}`
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, DiagramUnderstanding: understanding, DiagramConfirmed: true, DiagramFingerprint: "fp", BudgetUSD: 1, SpentUSD: &zero}}
	half := &llmclient.CreationStepResponse{Outcome: "draft", Message: "草稿", Brief: "b", DiagramUnderstanding: understanding, Draft: &llmclient.GeneratedSkill{Name: "x", Body: "1. 收到報帳申請\n2. 送經理簽核"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, half)
	if err != nil || !next || state != "queued" || e.Snapshot.Draft != nil {
		t.Fatalf("half a flow was accepted: state=%q next=%v err=%v", state, next, err)
	}
	if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "寄出付款通知") || strings.Contains(last.Content, "收到報帳申請") {
		t.Fatalf("the missing node was not named, or a present one was: %+v", last)
	}
	full := &llmclient.CreationStepResponse{Outcome: "draft", Message: "草稿", Brief: "b", DiagramUnderstanding: understanding, Draft: &llmclient.GeneratedSkill{Name: "x", Body: "1. 收到報帳申請。\n2. 送經理簽核。\n3. 寄出「付款通知」。"}}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, full)
	if err != nil || next || state != "draft_ready" || e.Snapshot.Draft == nil {
		t.Fatalf("a full walk was refused: state=%q next=%v err=%v", state, next, err)
	}
}

func TestRunUnmetReadsOnlyAFinishedEvaluation(t *testing.T) {
	cases := map[string]bool{
		`{"evaluation":{"evaluation_available":true,"status":"completed","overall":"partially_met"}}`: true,
		`{"evaluation":{"evaluation_available":true,"status":"completed","overall":"not_met"}}`:       true,
		`{"evaluation":{"evaluation_available":true,"status":"completed","overall":"met"}}`:           false,
		`{"evaluation":{"evaluation_available":true,"status":"failed","overall":"undetermined"}}`:     false,
		`{"evaluation":{"evaluation_available":false}}`:                                               false,
		`not json`: false,
	}
	for in, want := range cases {
		if got := runUnmet(in); got != want {
			t.Fatalf("runUnmet(%s) = %v, want %v", in, got, want)
		}
	}
}

// The same blocking validation report three times in a row hands the turn to
// the person instead of a fourth paid attempt (run h, 2026-09-06).
func TestProposalStopsARepeatedBlockedValidation(t *testing.T) {
	s := &Service{ValidateDraft: func(_ context.Context, d llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h-" + d.Body, "套件結構無法通過驗證：a second SKILL.md", true, nil
	}}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	for i, body := range []string{"one", "two"} {
		r := &llmclient.CreationStepResponse{Outcome: "draft", Message: "fixed", Brief: "b", Draft: &llmclient.GeneratedSkill{Name: "x", Body: body}}
		state, _, err := s.proposal(context.Background(), identity.Workspace{}, int64(2+i), &e, r)
		if err != nil || state != "draft_ready" || e.Snapshot.BlockedRepeats != i {
			t.Fatalf("attempt %d: state=%q repeats=%d err=%v", i+1, state, e.Snapshot.BlockedRepeats, err)
		}
	}
	r := &llmclient.CreationStepResponse{Outcome: "draft", Message: "fixed again", Brief: "b", Draft: &llmclient.GeneratedSkill{Name: "x", Body: "three"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil || next || state != "waiting_input" || e.Snapshot.BlockedRepeats != MaxBlockedRepeats || e.Snapshot.Draft == nil || !e.Snapshot.Draft.Blocked {
		t.Fatalf("third identical verdict must hand the turn back with the draft kept: state=%q next=%v repeats=%d err=%v", state, next, e.Snapshot.BlockedRepeats, err)
	}
	if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "assistant" || !strings.Contains(last.Content, "連續三次") {
		t.Fatalf("the person was not told: %+v", last)
	}
	// A different report resets the count.
	s.ValidateDraft = func(_ context.Context, d llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h-" + d.Body, "another", true, nil
	}
	state, _, err = s.proposal(context.Background(), identity.Workspace{}, 5, &e, r)
	if err != nil || state != "draft_ready" || e.Snapshot.BlockedRepeats != 0 {
		t.Fatalf("a new report is a new problem: state=%q repeats=%d err=%v", state, e.Snapshot.BlockedRepeats, err)
	}
}

// Without an uploaded diagram there is nothing to confirm: an interpretation
// the model volunteers for a text or reference session is dropped, not turned
// into a confirmation the person must click through (run h, 2026-09-06).
func TestProposalIgnoresADiagramInterpretationWhenNoDiagramWasUploaded(t *testing.T) {
	s := &Service{}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "clarification", Message: "請先確認流程圖的理解。", Brief: "b", DiagramUnderstanding: `{"nodes":["invented"],"conditions":[],"branches":[],"uncertainties":[]}`}
	state, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || state != "waiting_input" || e.Snapshot.DiagramUnderstanding != "" || e.Snapshot.PendingAction == "confirm_diagram" {
		t.Fatalf("an invented diagram became a confirmation: state=%q snap=%+v err=%v", state, e.Snapshot, err)
	}
}

// Re-validating the already validated, unblocked, unchanged draft ends the
// turn as draft_ready instead of another paid model call (run j, 2026-09-06).
func TestProposalDoesNotRevalidateTheSameAcceptedDraft(t *testing.T) {
	s := &Service{ValidateDraft: func(_ context.Context, d llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h-" + d.Body, "{}", false, nil
	}}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "tool_intent", Message: "validate", Brief: "b", ToolIntent: &llmclient.CreationToolIntent{Kind: "validate_draft"}, Draft: &llmclient.GeneratedSkill{Name: "x", Body: "same"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || !next || state != "queued" {
		t.Fatalf("first validation must queue the model: state=%q next=%v err=%v", state, next, err)
	}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || next || state != "draft_ready" || e.Snapshot.ToolCalls != 2 {
		t.Fatalf("second validation of the same draft must hand back draft_ready: state=%q next=%v tools=%d err=%v", state, next, e.Snapshot.ToolCalls, err)
	}
	if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "草稿就緒") {
		t.Fatalf("the model was not told: %+v", last)
	}
}

// A draft that arrives without a message is stored as a draft (run n,
// 2026-09-06: two sessions failed on the empty sentence, not on the draft).
func TestProposalAcceptsADraftWithAnEmptyMessage(t *testing.T) {
	s := &Service{ValidateDraft: func(_ context.Context, d llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h-" + d.Body, "{}", false, nil
	}}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "draft", Message: "", Brief: "b", Draft: &llmclient.GeneratedSkill{Name: "x", Body: "body"}}
	state, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || state != "draft_ready" || e.Snapshot.Draft == nil {
		t.Fatalf("an empty message beside a draft must not fail the step: state=%q err=%v", state, err)
	}
	if r.Message == "" {
		t.Fatal("the person still needs a sentence")
	}
}

// A fetch_url intent asks the person before anything connects (05 R-47).
func TestProposalHoldsAFetchUntilThePersonConfirms(t *testing.T) {
	s := &Service{Fetch: func(context.Context, string) (Fetch, string) {
		t.Fatal("nothing may be fetched at proposal time")
		return Fetch{}, ""
	}}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "tool_intent", Message: "查一下", ToolIntent: &llmclient.CreationToolIntent{Kind: "fetch_url", Query: " https://example.com/docs#top "}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != "waiting_confirmation" || e.Snapshot.PendingAction != "confirm_fetch" || e.Snapshot.PendingFetchURL != "https://example.com/docs" {
		t.Fatalf("state=%q next=%v pending=%q url=%q err=%v", state, next, e.Snapshot.PendingAction, e.Snapshot.PendingFetchURL, err)
	}
	// A private address never reaches the person as a question.
	r.ToolIntent.Query = "http://10.0.0.1/admin"
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || !next || state != "queued" {
		t.Fatalf("a refused URL must go back to the model: state=%q next=%v err=%v", state, next, err)
	}
	if last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "不符合規則") {
		t.Fatalf("the model was not told: %+v", last)
	}
}

// The questions after an unmet trial name each criterion the judge did not
// pass and the judge's reason; a passed trial asks nothing.
func TestTrialQuestionsNameTheFailedCriteria(t *testing.T) {
	obs := `{"evaluation":{"evaluation_available":true,"status":"completed","overall":"partially_met","criterion_results":[{"text":"輸出是核取方塊清單","result":"failed","reason":"輸出是表格"},{"text":"三條待辦","result":"passed"},{"text":"超過七天的分支","result":"undetermined","reason":"樣本沒有這個情境"}]}}`
	q := trialQuestions(obs)
	for _, want := range []string{"「輸出是核取方塊清單」：沒過——輸出是表格", "「超過七天的分支」：這份樣本驗不到——樣本沒有這個情境", "改草稿、還是改條件或範例輸入"} {
		if !strings.Contains(q, want) {
			t.Fatalf("missing %q in:\n%s", want, q)
		}
	}
	if strings.Contains(q, "三條待辦") {
		t.Fatalf("a passed criterion is not a question:\n%s", q)
	}
	if trialQuestions(strings.Replace(obs, `"result":"failed"`, `"result":"passed"`, 1)) != "" && trialQuestions(`{"evaluation":{"evaluation_available":false}}`) != "" {
		t.Fatal("nothing to ask must be empty")
	}
}

// search_knowledge takes the same road as search_catalog (references to
// confirm), only the ranking differs; it is offered only when wired.
func TestProposalRoutesSearchKnowledgeToTheSemanticSearch(t *testing.T) {
	semantic := 0
	s := &Service{
		SearchReferences: func(context.Context, identity.Workspace, string) ([]Reference, error) {
			t.Fatal("lexical search must not run")
			return nil, nil
		},
		SearchKnowledge: func(_ context.Context, _ identity.Workspace, q string) ([]Reference, error) {
			semantic++
			return []Reference{{SkillID: "s1", VersionID: "v1", Name: "found", Available: true}}, nil
		},
	}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, BudgetUSD: 1, SpentUSD: &zero}}
	r := &llmclient.CreationStepResponse{Outcome: "tool_intent", Message: "找相近的", ToolIntent: &llmclient.CreationToolIntent{Kind: "search_knowledge", Query: "把會議逐字稿整理成待辦"}}
	state, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || state != "waiting_confirmation" || e.Snapshot.PendingAction != "confirm_references" || semantic != 1 || len(e.Snapshot.References) != 1 {
		t.Fatalf("state=%q pending=%q semantic=%d refs=%d err=%v", state, e.Snapshot.PendingAction, semantic, len(e.Snapshot.References), err)
	}
	if got := allowedTools(0, 8, false, true); len(got) != 3 || got[2] != "search_knowledge" {
		t.Fatalf("search_knowledge is offered only when wired: %v", got)
	}
	if got := allowedTools(0, 8, false, false); len(got) != 2 {
		t.Fatalf("not wired, not offered: %v", got)
	}
}
