package creation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
)

func TestEvaluationCasesNeedTwoDifferentValidInputs(t *testing.T) {
	duplicate := pairedEvaluationCases()
	duplicate[1].Prompt = duplicate[0].Prompt
	for _, tc := range []struct {
		name  string
		cases []ChallengeCase
		want  bool
	}{
		{"none", nil, false},
		{"one", pairedEvaluationCases()[:1], false},
		{"two", pairedEvaluationCases(), true},
		{"three", append(pairedEvaluationCases(), aChallengeCase()), true},
		{"four", append(pairedEvaluationCases(), aChallengeCase(), ChallengeCase{Name: "另一份摘要", Prompt: "請整理更新：會議改為後天出貨。", Criteria: []string{"列出後天出貨的更新。"}, ExpectedInteraction: "direct_answer"}), false},
		{"same input", duplicate, false},
		{"blank criterion", []ChallengeCase{aChallengeCase(), {Name: "second", Prompt: "other", Criteria: []string{" "}, ExpectedInteraction: "direct_answer"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := completeEvaluationCases(tc.cases); got != tc.want {
				t.Fatalf("completeEvaluationCases = %v, want %v", got, tc.want)
			}
		})
	}
	duplicate = pairedEvaluationCases()
	duplicate[1].Prompt = " \n" + duplicate[0].Prompt + "\t"
	if completeEvaluationCases(duplicate) {
		t.Fatal("surrounding whitespace cannot make a second input")
	}
}

func TestEvaluationCasesRequireBothKnownInteractionKinds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kinds []string
		want  bool
	}{
		{"both", []string{"question", "direct_answer"}, true},
		{"reverse", []string{"direct_answer", "question"}, true},
		{"questions only", []string{"question", "question"}, false},
		{"answers only", []string{"direct_answer", "direct_answer"}, false},
		{"legacy", []string{"", ""}, false},
		{"one missing", []string{"question", ""}, false},
		{"unknown", []string{"question", "other"}, false},
		{"unclassified extra", []string{"question", "direct_answer", ""}, false},
		{"unsupported extra", []string{"question", "direct_answer", "other"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases := append(pairedEvaluationCases(), aChallengeCase())[:len(tc.kinds)]
			for i, kind := range tc.kinds {
				cases[i].ExpectedInteraction = kind
			}
			if got := completeEvaluationCases(cases); got != tc.want {
				t.Fatalf("classifications %v: complete=%v, want %v", tc.kinds, got, tc.want)
			}
		})
	}
}

func TestUnsupportedStoredClassificationsBecomeUnknownInTheProviderRequest(t *testing.T) {
	cases := pairedEvaluationCases()
	cases[0].ExpectedInteraction = "future"
	wire := wireChallengeCases(cases)
	if wire[0].ExpectedInteraction != "" || wire[1].ExpectedInteraction != "direct_answer" || cases[0].ExpectedInteraction != "future" {
		t.Fatalf("wire=%+v stored=%+v, want unknown metadata omitted without changing the stored cases", wire, cases)
	}
	if wire[0].Name != "缺少輸入" || wire[0].Prompt != "請整理摘要。" || !reflect.DeepEqual(wire[0].Criteria, []string{"詢問要整理的原文。"}) {
		t.Fatalf("repair request lost the legacy case content: %+v", wire[0])
	}
}

func TestAnIncompleteEvaluationProposalCannotReplaceTheConfirmedRequirements(t *testing.T) {
	for _, cases := range [][]ChallengeCase{nil, pairedEvaluationCases()[:1], {aChallengeCase(), aChallengeCase()}} {
		e := fundedSession(Snapshot{Brief: "original", BriefConfirmed: true, AcceptanceCriteria: []string{"original criterion"}, SampleInput: "original input", ChallengeCases: pairedEvaluationCases()})
		r := &StepResult{Outcome: outcomeConfirmBrief, Message: "confirm", Brief: "replacement", AcceptanceCriteria: []string{"replacement criterion"}, SampleInput: "replacement input", ChallengeCases: cases}
		state, next, err := (&Service{}).proposal(context.Background(), 3, &e, r, nil)
		if err != nil || next || state != StateWaitingInput || e.Snapshot.PendingAction != NothingPending {
			t.Fatalf("incomplete proposal state=%s next=%v err=%v", state, next, err)
		}
		if e.Snapshot.Brief != "original" || !e.Snapshot.BriefConfirmed || e.Snapshot.SampleInput != "original input" || !reflect.DeepEqual(e.Snapshot.AcceptanceCriteria, []string{"original criterion"}) || !reflect.DeepEqual(e.Snapshot.ChallengeCases, pairedEvaluationCases()) {
			t.Fatalf("incomplete proposal replaced confirmed data: %+v", e.Snapshot)
		}
		if !strings.Contains(lastMessage(e.Snapshot), "評估案例") || e.Snapshot.Draft != nil || e.Snapshot.ToolCalls != 0 {
			t.Fatalf("incomplete proposal did not name its missing cases: %+v", e.Snapshot)
		}
	}
}

func TestADraftWithoutCompleteEvaluationCasesIsNotValidatedOrStored(t *testing.T) {
	for name, cases := range map[string][]ChallengeCase{
		"none": nil, "one": pairedEvaluationCases()[:1],
		"duplicate": {aChallengeCase(), aChallengeCase()},
	} {
		for entry, as := range map[string]func(*StepResult) *StepResult{"draft": asDraft, "validate_draft": asValidation} {
			t.Run(name+"/"+entry, func(t *testing.T) {
				p := Snapshot{Brief: "original", BriefConfirmed: true, ChallengeCases: cases}
				r := as(&StepResult{Message: "ready", Draft: &GeneratedSkill{Body: "new body"}})
				got, after := screened(t, p, r)
				if got.state != StateWaitingInput || got.validated != 0 || after.Draft != nil || after.ToolCalls != 0 || !after.BriefConfirmed {
					t.Fatalf("incomplete draft got=%+v snapshot=%+v, want no validation or stored draft", got, after)
				}
				if !strings.Contains(got.message, "評估案例") {
					t.Fatalf("missing cases were not explained: %q", got.message)
				}
			})
		}
	}
}

func TestALegacyBriefCanConfirmNewEvaluationCasesWithoutChangingItsText(t *testing.T) {
	e := fundedSession(Snapshot{Brief: "original", BriefConfirmed: true})
	r := &StepResult{Outcome: outcomeConfirmBrief, Message: "confirm the evaluation cases", ChallengeCases: pairedEvaluationCases()}
	state, next, err := (&Service{}).proposal(context.Background(), 3, &e, r, nil)
	if err != nil || next || state != StateWaitingConfirmation || e.Snapshot.BriefConfirmed || e.Snapshot.PendingAction != PendingBriefConfirmation || e.Snapshot.Brief != "original" || !reflect.DeepEqual(e.Snapshot.ChallengeCases, pairedEvaluationCases()) {
		t.Fatalf("legacy case proposal state=%s next=%v snapshot=%+v err=%v", state, next, e.Snapshot, err)
	}
}

func TestConfirmingALegacyBriefRepairsItsCasesBeforeMarkingItConfirmed(t *testing.T) {
	p := Snapshot{Brief: "original", PendingAction: PendingBriefConfirmation, ChallengeCases: pairedEvaluationCases()[:1]}
	outcome, err := confirmBrief(&p)
	if err != nil || !outcome.queueStep || p.BriefConfirmed || p.PendingAction != NothingPending || !strings.Contains(lastMessage(p), "評估案例") {
		t.Fatalf("legacy confirmation outcome=%+v snapshot=%+v err=%v", outcome, p, err)
	}
}

func TestALegacyDraftCannotMaterializeBeforeItsEvaluationCasesAreConfirmed(t *testing.T) {
	for _, kind := range []string{commandMaterialize, commandFinalize, commandConfirmDuplicate} {
		t.Run(kind, func(t *testing.T) {
			p := saveableSnapshot()
			p.ChallengeCases = pairedEvaluationCases()[:1]
			if kind == commandConfirmDuplicate {
				p.PendingAction, p.PendingMaterialize = PendingDuplicateAcknowledgement, commandMaterialize
			}
			draft := p.Draft
			outcome, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: kind, ContentHash: draft.ContentHash})
			if err != nil || !outcome.queueStep || outcome.materialize != "" || p.BriefConfirmed || p.Candidate != nil || p.Draft != draft || !strings.Contains(lastMessage(p), "評估案例") {
				t.Fatalf("legacy save outcome=%+v snapshot=%+v err=%v", outcome, p, err)
			}
		})
	}
}

func TestAnExistingCandidateCanStillBeSelectedWithoutCreatingAnotherVersion(t *testing.T) {
	p := saveableSnapshot()
	p.ChallengeCases = nil
	p.Candidate = &Candidate{SkillID: "existing", VersionID: "immutable"}
	outcome, err := (&Service{}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: commandFinalize, ContentHash: p.Draft.ContentHash})
	if err != nil || outcome.state != StateSaved || outcome.queueStep || outcome.materialize != "" || p.Candidate.VersionID != "immutable" {
		t.Fatalf("existing candidate outcome=%+v snapshot=%+v err=%v", outcome, p, err)
	}
}

func TestEvaluationCasesAreCreatedEvenWithoutTheSeparateAcceptanceCase(t *testing.T) {
	var prompts []string
	s := &Service{CreateAcceptanceTestCase: func(_ context.Context, _ pgx.Tx, _ identity.Workspace, _, _, prompt string, _ []string) (string, error) {
		prompts = append(prompts, prompt)
		return prompt, nil
	}}
	p := Snapshot{ChallengeCases: pairedEvaluationCases()}
	candidate := Candidate{SkillID: "skill"}
	if err := s.attachAcceptanceTestCase(context.Background(), nil, identity.Workspace{}, p, &candidate); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prompts, []string{p.ChallengeCases[0].Prompt, p.ChallengeCases[1].Prompt}) || len(candidate.ChallengeTestCaseIDs) != 2 || candidate.TestCaseID != "" {
		t.Fatalf("prompts=%v candidate=%+v, want both evaluation cases", prompts, candidate)
	}
}

func TestAMissingTestCaseWriterRefusesTheCandidate(t *testing.T) {
	err := (&Service{}).attachAcceptanceTestCase(context.Background(), nil, identity.Workspace{}, Snapshot{ChallengeCases: pairedEvaluationCases()}, &Candidate{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing case writer = %v, want ErrUnavailable", err)
	}
}
