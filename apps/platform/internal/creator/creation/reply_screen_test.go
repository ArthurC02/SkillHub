package creation

import (
	"context"
	"reflect"
	"testing"
)

type screenOutcome struct {
	state     State
	pending   PendingAction
	message   string
	validated int
}

func fundedSession(p Snapshot) envelope {
	zero := 0.0
	p.BudgetUSD, p.SpentUSD = 1, &zero
	return envelope{Limits: testLimitsForProposal(), Snapshot: p}
}

func countingValidator(calls *int, blocked bool) *Service {
	return &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		*calls++
		return "hash", "report", blocked, nil
	}}
}

func lastMessage(p Snapshot) string {
	if len(p.Messages) == 0 {
		return ""
	}
	return p.Messages[len(p.Messages)-1].Content
}

func screened(t *testing.T, p Snapshot, r *StepResult) (screenOutcome, Snapshot) {
	t.Helper()
	calls := 0
	e := fundedSession(p)
	state, _, err := countingValidator(&calls, false).proposal(context.Background(), 4, &e, r, nil)
	if err != nil {
		t.Fatalf("the screen let a reply through to a hard refusal: %v", err)
	}
	return screenOutcome{state, e.Snapshot.PendingAction, lastMessage(e.Snapshot), calls}, e.Snapshot
}

func asDraft(r *StepResult) *StepResult {
	r.Outcome = outcomeDraft
	return r
}

func asValidation(r *StepResult) *StepResult {
	r.Outcome, r.ToolIntent = outcomeToolIntent, &ToolIntent{Kind: "validate_draft"}
	return r
}

func withDiagram(change func(*Snapshot)) Snapshot {
	p := Snapshot{Brief: "task", BriefConfirmed: true}
	attachConfirmedDiagram(&p)
	change(&p)
	return p
}

const (
	confirmBriefFirstSentence   = "請先確認這份需求與驗收條件，再建立草稿。"
	confirmDiagramFirstSentence = "請先確認流程圖的理解；確認後再依它建立草稿。"
	draftRetrySentence          = "模型這一步沒有交出草稿，已自動再試一次。"
)

func TestADraftOutOfTurnIsPutBackToThePersonWithoutValidating(t *testing.T) {
	cases := map[string]struct {
		session Snapshot
		reply   func() *StepResult
		want    screenOutcome
	}{
		"brief never confirmed": {
			Snapshot{Brief: "task"},
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingConfirmation, PendingBriefConfirmation, confirmBriefFirstSentence, 0},
		},
		"confirmed brief changed by the model": {
			Snapshot{Brief: "task", BriefConfirmed: true},
			func() *StepResult {
				return &StepResult{Message: "ok", Brief: "another task", Draft: &GeneratedSkill{Body: "b"}}
			},
			screenOutcome{StateWaitingConfirmation, PendingBriefConfirmation, confirmBriefFirstSentence, 0},
		},
		"no draft in the reply": {
			Snapshot{Brief: "task", BriefConfirmed: true},
			func() *StepResult { return &StepResult{Message: "ok"} },
			screenOutcome{StateQueued, NothingPending, draftRetrySentence, 0},
		},
		"diagram description read back but not confirmed": {
			withDiagram(func(p *Snapshot) {
				p.DiagramDescriptionConfirmed, p.DiagramConfirmed, p.DiagramInterpretation = false, false, nil
			}),
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingConfirmation, PendingDiagramDescription, confirmDiagramFirstSentence, 0},
		},
		"diagram uncertainty still unanswered": {
			withDiagram(func(p *Snapshot) {
				p.DiagramConfirmed = false
				p.DiagramInterpretation.Uncertainties[0].Answer = ""
			}),
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingConfirmation, PendingDiagramAnswers, confirmDiagramFirstSentence, 0},
		},
		"diagram answered but not confirmed": {
			withDiagram(func(p *Snapshot) { p.DiagramConfirmed = false }),
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingConfirmation, PendingDiagramInterpretation, confirmDiagramFirstSentence, 0},
		},
		"diagram uploaded and never described": {
			withDiagram(func(p *Snapshot) {
				p.DiagramDescription, p.DiagramDescriptionConfirmed, p.DiagramConfirmed, p.DiagramInterpretation = "", false, false, nil
			}),
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingInput, NothingPending, confirmDiagramFirstSentence, 0},
		},
		"legacy diagram confirmed without a structured reading": {
			Snapshot{Brief: "task", BriefConfirmed: true, DiagramFingerprint: "fp", DiagramConfirmed: true, DiagramUnderstanding: diagramJSON("start")},
			func() *StepResult { return &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}} },
			screenOutcome{StateWaitingInput, NothingPending, confirmDiagramFirstSentence, 0},
		},
	}
	for name, tc := range cases {
		for entry, as := range map[string]func(*StepResult) *StepResult{"draft": asDraft, "validate_draft": asValidation} {
			t.Run(name+"/"+entry, func(t *testing.T) {
				got, after := screened(t, tc.session, as(tc.reply()))
				if got != tc.want {
					t.Fatalf("got %+v, want %+v", got, tc.want)
				}
				if after.Draft != nil || after.ToolCalls != 0 {
					t.Fatalf("a refused draft was stored or charged: draft=%v toolCalls=%d", after.Draft, after.ToolCalls)
				}
			})
		}
	}
}

func TestAChangedBriefWithADraftReopensTheBriefWithEverythingTheModelProposed(t *testing.T) {
	session := Snapshot{Brief: "task", BriefConfirmed: true, AcceptanceCriteria: []string{"c"}, SampleInput: "s"}
	_, after := screened(t, session, asDraft(&StepResult{Message: "ok", Brief: "another task", AcceptanceCriteria: []string{"d"}, SampleInput: "t", Draft: &GeneratedSkill{Body: "b"}}))
	if after.Brief != "another task" || !reflect.DeepEqual(after.AcceptanceCriteria, []string{"d"}) || after.SampleInput != "t" || after.BriefConfirmed {
		t.Fatalf("snapshot = %+v, want the proposal put up for confirmation", after)
	}
}

func TestAConfirmedBriefIgnoresChangesTheModelDidNotPutUpForConfirmation(t *testing.T) {
	session := Snapshot{Brief: "task", BriefConfirmed: true, AcceptanceCriteria: []string{"c"}, SampleInput: "s"}
	cases := map[string]*StepResult{
		"a question":                       {Outcome: outcomeClarification, Message: "?", Brief: "another task", AcceptanceCriteria: []string{"d"}, SampleInput: "t"},
		"a draft that keeps the brief":     {Outcome: outcomeDraft, Message: "ok", AcceptanceCriteria: []string{"d"}, SampleInput: "t", Draft: &GeneratedSkill{Body: "b"}},
		"a fetch that rewrites the inputs": {Outcome: outcomeToolIntent, Message: "看", Brief: "another task", ToolIntent: &ToolIntent{Kind: "fetch_url", Query: "https://x.test"}},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			p := session
			e := fundedSession(p)
			calls := 0
			s := countingValidator(&calls, false)
			s.Fetch = func(context.Context, string) (Fetch, string) { return Fetch{}, "" }
			if _, _, err := s.proposal(context.Background(), 4, &e, r, nil); err != nil {
				t.Fatal(err)
			}
			if e.Snapshot.Brief != "task" || !reflect.DeepEqual(e.Snapshot.AcceptanceCriteria, []string{"c"}) || e.Snapshot.SampleInput != "s" || !e.Snapshot.BriefConfirmed {
				t.Fatalf("snapshot = %+v, want the confirmed inputs kept", e.Snapshot)
			}
		})
	}
}

func TestAnUnconfirmedBriefTakesWhatTheModelProposesWithAnyOutcome(t *testing.T) {
	_, after := screened(t, Snapshot{Brief: "task"}, &StepResult{Outcome: outcomeClarification, Message: "?", Brief: "another task"})
	if after.Brief != "another task" || after.PendingAction != PendingBriefConfirmation {
		t.Fatalf("snapshot = %+v, want the proposal put up for confirmation", after)
	}
}

func TestAskingToConfirmABriefNeedsABriefAndAChange(t *testing.T) {
	cases := map[string]struct {
		session Snapshot
		reply   *StepResult
		want    State
	}{
		"nothing proposed and nothing kept":  {Snapshot{}, &StepResult{Outcome: outcomeConfirmBrief, Message: "ok"}, StateQueued},
		"only spaces proposed":               {Snapshot{Brief: "task"}, &StepResult{Outcome: outcomeConfirmBrief, Message: "ok", Brief: "   "}, StateQueued},
		"the confirmed brief restated":       {Snapshot{Brief: "task", BriefConfirmed: true, SampleInput: "s"}, &StepResult{Outcome: outcomeConfirmBrief, Message: "ok", Brief: "task", SampleInput: "s"}, StateQueued},
		"a new sample for a confirmed brief": {Snapshot{Brief: "task", BriefConfirmed: true, SampleInput: "s"}, &StepResult{Outcome: outcomeConfirmBrief, Message: "ok", SampleInput: "s, and a missing case"}, StateWaitingConfirmation},
		"a kept brief not yet confirmed":     {Snapshot{Brief: "task"}, &StepResult{Outcome: outcomeConfirmBrief, Message: "ok"}, StateWaitingConfirmation},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, _ := screened(t, tc.session, tc.reply)
			if got.state != tc.want {
				t.Fatalf("state = %q, want %q", got.state, tc.want)
			}
		})
	}
}

func TestACleanDraftIsReadyAfterOneValidationAndNoToolCall(t *testing.T) {
	got, after := screened(t, Snapshot{Brief: "task", BriefConfirmed: true}, asDraft(&StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}}))
	if got.state != StateDraftReady || got.validated != 1 || after.ToolCalls != 0 || after.Draft == nil || after.Draft.Blocked {
		t.Fatalf("got %+v toolCalls=%d draft=%+v, want it ready after one validation", got, after.ToolCalls, after.Draft)
	}
}

func TestABlockedDraftGoesBackToTheModelUntilTheToolCallsRunOut(t *testing.T) {
	limits := testLimitsForProposal()
	for spent, want := range map[int]State{limits.MaxToolCalls - 1: StateQueued, limits.MaxToolCalls: StateDraftReady} {
		calls := 0
		e := fundedSession(Snapshot{Brief: "task", BriefConfirmed: true, ToolCalls: spent})
		state, _, err := countingValidator(&calls, true).proposal(context.Background(), 4, &e, asDraft(&StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}}), nil)
		if err != nil || state != want || e.Snapshot.Draft == nil || !e.Snapshot.Draft.Blocked {
			t.Fatalf("after %d tool calls: state=%q draft=%+v err=%v, want %q with the blocked draft kept", spent, state, e.Snapshot.Draft, err, want)
		}
		if want == StateQueued && e.Snapshot.ToolCalls != spent+1 {
			t.Fatalf("returning to the model spent %d tool calls, want %d", e.Snapshot.ToolCalls, spent+1)
		}
	}
}

func TestADraftThatReplacesAnotherKeepsTheReplacedOneAsPrevious(t *testing.T) {
	calls := 0
	stored := &Draft{ContentHash: "older", Skill: GeneratedSkill{Body: "older body"}}
	e := fundedSession(Snapshot{Brief: "task", BriefConfirmed: true, Draft: stored})
	if _, _, err := countingValidator(&calls, false).proposal(context.Background(), 4, &e, asDraft(&StepResult{Message: "ok", Draft: &GeneratedSkill{Body: "b"}}), nil); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot.PreviousDraft == nil || e.Snapshot.PreviousDraft.ContentHash != "older" {
		t.Fatalf("previous draft %+v, want the replaced one", e.Snapshot.PreviousDraft)
	}
}

func TestAskingToValidateWithoutADraftValidatesTheStoredOne(t *testing.T) {
	var seen string
	s := &Service{ValidateDraft: func(_ context.Context, d GeneratedSkill) (string, string, bool, error) {
		seen = d.Body
		return "h", "report", false, nil
	}}
	e := fundedSession(Snapshot{Brief: "task", BriefConfirmed: true, Draft: &Draft{ContentHash: "old", Skill: GeneratedSkill{Body: "stored body"}}})
	if _, _, err := s.proposal(context.Background(), 4, &e, asValidation(&StepResult{Message: "ok"}), nil); err != nil || seen != "stored body" {
		t.Fatalf("validated %q, err %v; want the stored draft", seen, err)
	}
}

func TestAWebAddressIsRecognisedWhateverItsCaseOrLeadingSpace(t *testing.T) {
	for query, want := range map[string]bool{"https://x.test": true, "  HTTP://x.test": true, "x.test": false, "ftp://x.test": false, "": false} {
		if got := namesAWebAddress(query); got != want {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
}
