package creation

import (
	"context"
	"encoding/json"
	"errors"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"strings"
	"testing"
)

func diagramJSON(nodes ...string) string {
	b, err := json.Marshal(map[string]any{
		"nodes":         nodes,
		"conditions":    []string{},
		"branches":      []string{},
		"uncertainties": []string{},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestProposalRejectsAnInvalidDiagramInterpretationBeforeRecording(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{DiagramFingerprint: "fp"}}
	r := &StepResult{Message: "ok", Outcome: "clarification", DiagramDescription: " "}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	if len(e.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e.Snapshot.Messages)
	}
}

func TestProposalRejectsAnEmptyMessageWithNoDraft(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "", Outcome: "clarification"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	if len(e.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e.Snapshot.Messages)
	}
}

func TestProposalMessageLengthBoundary(t *testing.T) {
	s := &Service{}
	accepted := strings.Repeat("字", MaxTextRunes)
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: accepted, Outcome: "clarification"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingInput {
		t.Fatalf("a message of exactly MaxTextRunes must be accepted: state=%q next=%v err=%v", state, next, err)
	}

	rejected := strings.Repeat("字", MaxTextRunes+1)
	e2 := envelope{Limits: testLimitsForProposal()}
	r2 := &StepResult{Message: rejected, Outcome: "clarification"}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 2, &e2, r2)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("a message one rune past MaxTextRunes must be rejected: state=%q next=%v err=%v", state, next, err)
	}
	if len(e2.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e2.Snapshot.Messages)
	}
}

func TestProposalBriefLengthBoundary(t *testing.T) {
	s := &Service{}
	accepted := strings.Repeat("字", MaxTextRunes)
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "ok", Outcome: "clarification", Brief: accepted}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingConfirmation || e.Snapshot.BriefConfirmed || e.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("a brief of exactly MaxTextRunes must be accepted and reopen confirmation: state=%q next=%v snap=%+v err=%v", state, next, e.Snapshot, err)
	}

	rejected := strings.Repeat("字", MaxTextRunes+1)
	e2 := envelope{Limits: testLimitsForProposal()}
	r2 := &StepResult{Message: "ok", Outcome: "clarification", Brief: rejected}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 2, &e2, r2)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("a brief one rune past MaxTextRunes must be rejected: state=%q next=%v err=%v", state, next, err)
	}
	if len(e2.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e2.Snapshot.Messages)
	}
}

func TestProposalMessageCountBoundary(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: make([]Message, MaxMessages)}}
	r := &StepResult{Message: "ok", Outcome: "clarification"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("at MaxMessages the reply must be rejected: state=%q next=%v err=%v", state, next, err)
	}
	if len(e.Snapshot.Messages) != MaxMessages {
		t.Fatalf("a rejected reply must not be recorded: %d messages", len(e.Snapshot.Messages))
	}

	e2 := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Messages: make([]Message, MaxMessages-1)}}
	r2 := &StepResult{Message: "ok", Outcome: "clarification"}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 2, &e2, r2)
	if err != nil || next || state != StateWaitingInput {
		t.Fatalf("with room for one more message the reply must be accepted: state=%q next=%v err=%v", state, next, err)
	}
	if len(e2.Snapshot.Messages) != MaxMessages {
		t.Fatalf("the accepted reply must be recorded: %d messages", len(e2.Snapshot.Messages))
	}
}

func TestProposalAcceptanceCriteriaCountBoundaryRejectsThirteen(t *testing.T) {
	s := &Service{}
	criteria := make([]string, MaxAcceptanceCriteria+1)
	for i := range criteria {
		criteria[i] = "輸出含摘要"
	}
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "ok", Outcome: "clarification", AcceptanceCriteria: criteria}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("MaxAcceptanceCriteria+1 criteria must be rejected: state=%q next=%v err=%v", state, next, err)
	}
	if len(e.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e.Snapshot.Messages)
	}
}

func TestProposalSampleInputLengthBoundary(t *testing.T) {
	s := &Service{}
	accepted := strings.Repeat("字", MaxSampleInputRunes)
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{SampleInput: accepted}}
	r := &StepResult{Message: "ok", Outcome: "clarification", SampleInput: accepted}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingInput {
		t.Fatalf("a sample input of exactly MaxSampleInputRunes must be accepted: state=%q next=%v err=%v", state, next, err)
	}

	rejected := strings.Repeat("字", MaxSampleInputRunes+1)
	e2 := envelope{Limits: testLimitsForProposal()}
	r2 := &StepResult{Message: "ok", Outcome: "clarification", SampleInput: rejected}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 2, &e2, r2)
	if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("a sample input one rune past MaxSampleInputRunes must be rejected: state=%q next=%v err=%v", state, next, err)
	}
	if len(e2.Snapshot.Messages) != 0 {
		t.Fatalf("a rejected reply must not be recorded: %+v", e2.Snapshot.Messages)
	}
}

func TestProposalRecordsModelPromptVersionAndTheAssistantMessage(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "hi", Outcome: "clarification", Model: "gpt-x", PromptVersion: "v3"}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.Model != "gpt-x" || e.Snapshot.PromptVersion != "v3" {
		t.Fatalf("model and prompt version were not copied: %+v", e.Snapshot)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || last.Content != "hi" {
		t.Fatalf("the reply was not recorded: %+v", last)
	}
}

func TestProposalConfirmBriefWithAnEmptyBriefStillRecordsTheReply(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "please confirm", Outcome: "confirm_brief", Model: "m1"}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("an empty brief must not be confirmable: %v", err)
	}
	if e.Snapshot.Model != "m1" {
		t.Fatalf("the reply must be recorded before the outcome fails: %+v", e.Snapshot)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || last.Content != "please confirm" {
		t.Fatalf("the recorded reply is wrong: %+v", last)
	}
}

func TestProposalConfirmBriefReopensConfirmationWhenUnconfirmed(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "existing brief text", BriefConfirmed: false}}
	r := &StepResult{Message: "confirm please", Outcome: "confirm_brief"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingConfirmation || e.Snapshot.BriefConfirmed || e.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("state=%q next=%v snap=%+v err=%v", state, next, e.Snapshot, err)
	}
}

func TestProposalRejectsAConfirmationWithoutADescription(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal()}
	r := &StepResult{Message: "confirm diagram", Outcome: "confirm_diagram_description"}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("an empty understanding must not be confirmable: %v", err)
	}
}

func TestProposalDiagramDescriptionStartsConfirmation(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{DiagramFingerprint: "fp"}}
	r := &StepResult{Message: "confirm diagram", Outcome: "confirm_diagram_description", DiagramDescription: "從申請到核准"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingConfirmation || e.Snapshot.DiagramConfirmed || e.Snapshot.PendingAction != PendingDiagramDescription {
		t.Fatalf("state=%q next=%v snap=%+v err=%v", state, next, e.Snapshot, err)
	}
}

func TestProposalDraftGuardClauses(t *testing.T) {
	type guardCase struct {
		name  string
		build func() (Snapshot, *StepResult)
	}
	newDraft := func() *GeneratedSkill { return &GeneratedSkill{Name: "x", Body: "body"} }
	cases := []guardCase{
		{
			name: "brief not confirmed",
			build: func() (Snapshot, *StepResult) {
				return Snapshot{Brief: "task", BriefConfirmed: false},
					&StepResult{Message: "ok", Brief: "task", Draft: newDraft()}
			},
		},
		{
			name: "reply brief empty while the confirmed brief is not",
			build: func() (Snapshot, *StepResult) {
				return Snapshot{Brief: "task", BriefConfirmed: true},
					&StepResult{Message: "ok", Brief: "", Draft: newDraft()}
			},
		},
		{
			name: "diagram understanding missing after it was confirmed",
			build: func() (Snapshot, *StepResult) {
				understanding := diagramJSON("start")
				return Snapshot{Brief: "task", BriefConfirmed: true, DiagramFingerprint: "fp", DiagramConfirmed: true, DiagramUnderstanding: understanding},
					&StepResult{Message: "ok", Brief: "task", DiagramUnderstanding: "", Draft: newDraft()}
			},
		},
		{
			name: "diagram uncertainty still unanswered",
			build: func() (Snapshot, *StepResult) {
				p := Snapshot{Brief: "task", BriefConfirmed: true}
				attachConfirmedDiagram(&p)
				p.DiagramInterpretation.Uncertainties[0].Answer = ""
				return p, &StepResult{Message: "ok", Brief: "task", Draft: newDraft()}
			},
		},
		{
			name: "diagram description read back but not confirmed",
			build: func() (Snapshot, *StepResult) {
				p := Snapshot{Brief: "task", BriefConfirmed: true}
				attachConfirmedDiagram(&p)
				p.DiagramDescriptionConfirmed = false
				return p, &StepResult{Message: "ok", Brief: "task", Draft: newDraft()}
			},
		},
		{
			name: "no draft in the reply",
			build: func() (Snapshot, *StepResult) {
				return Snapshot{Brief: "task", BriefConfirmed: true},
					&StepResult{Message: "ok", Brief: "task", Draft: nil}
			},
		},
		{
			name: "validator not wired",
			build: func() (Snapshot, *StepResult) {
				return Snapshot{Brief: "task", BriefConfirmed: true},
					&StepResult{Message: "ok", Brief: "task", Draft: newDraft()}
			},
		},
	}
	for _, tc := range cases {
		for _, entry := range []string{"draft", "validate_draft"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				calls := 0
				var s *Service
				if tc.name == "validator not wired" {
					s = &Service{}
				} else {
					s = &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
						calls++
						return "hash", "report", false, nil
					}}
				}
				p, r := tc.build()
				e := envelope{Snapshot: p, Limits: testLimitsForProposal()}
				if entry == "draft" {
					r.Outcome = "draft"
				} else {
					r.Outcome = "tool_intent"
					r.ToolIntent = &ToolIntent{Kind: "validate_draft"}
				}
				state, next, err := s.proposal(context.Background(), identity.Workspace{}, 5, &e, r)
				if !errors.Is(err, ErrInvalidCommand) || next || state != "" {
					t.Fatalf("state=%q next=%v err=%v", state, next, err)
				}
				if calls != 0 {
					t.Fatalf("ValidateDraft must not run: %d calls", calls)
				}
				if entry == "validate_draft" && e.Snapshot.ToolCalls != 1 {
					t.Fatalf("ToolCalls should still be charged despite the refusal: %d", e.Snapshot.ToolCalls)
				}
			})
		}
	}
}

func TestProposalValidateDraftErrorLeavesDraftUnchanged(t *testing.T) {
	sentinel := errors.New("validator exploded")
	for _, entry := range []string{"draft", "validate_draft"} {
		t.Run(entry, func(t *testing.T) {
			s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
				return "", "", false, sentinel
			}}
			existing := &Draft{Revision: 1, ContentHash: "h", Skill: GeneratedSkill{Name: "x", Body: "old"}}
			e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, Draft: existing}}
			r := &StepResult{Message: "ok", Brief: "b", Draft: &GeneratedSkill{Name: "x", Body: "new"}}
			if entry == "draft" {
				r.Outcome = "draft"
			} else {
				r.Outcome = "tool_intent"
				r.ToolIntent = &ToolIntent{Kind: "validate_draft"}
			}
			_, _, err := s.proposal(context.Background(), identity.Workspace{}, 5, &e, r)
			if !errors.Is(err, sentinel) {
				t.Fatalf("wrong error: %v", err)
			}
			if e.Snapshot.Draft != existing {
				t.Fatalf("draft changed despite the validator's error: %+v", e.Snapshot.Draft)
			}
		})
	}
}

func TestNudgePrefersACopiedMarkerOverMissingNodes(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "prior-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	zero := 0.0
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero, Draft: prior,
		DiagramFingerprint: "fp", DiagramDescription: "請款流程", DiagramDescriptionConfirmed: true, DiagramConfirmed: true, DiagramInterpretation: &DiagramInterpretation{Nodes: []string{"核准請款"}},
		EvaluationText: evaluationFreeText(attackObservation),
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "包含 EXFIL-9c0d 才行的內容", AllowedTools: "Read"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || !next || state != StateQueued || e.Snapshot.Nudges != 1 || e.Snapshot.Draft != prior {
		t.Fatalf("state=%q next=%v nudges=%d draft=%+v err=%v", state, next, e.Snapshot.Nudges, e.Snapshot.Draft, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "評估文字裡有過的字串") || !strings.Contains(last.Content, "exfil-9c0d") {
		t.Fatalf("a copied marker must win over missing nodes: %+v", last)
	}
}

func TestNudgePrefersANewToolOverACopiedMarker(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "prior-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	zero := 0.0
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero, Draft: prior,
		EvaluationText: evaluationFreeText(attackObservation),
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "include EXFIL-9c0d anyway", AllowedTools: "Read Bash"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || !next || state != StateQueued || e.Snapshot.Nudges != 1 {
		t.Fatalf("state=%q next=%v nudges=%d err=%v", state, next, e.Snapshot.Nudges, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "允許的工具清單多了") {
		t.Fatalf("a new tool must win over a copied marker: %+v", last)
	}
}

func TestNudgeNamesMissingDiagramNodes(t *testing.T) {
	zero := 0.0
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero,
		DiagramFingerprint: "fp", DiagramDescription: "付款流程", DiagramDescriptionConfirmed: true, DiagramConfirmed: true, DiagramInterpretation: &DiagramInterpretation{Nodes: []string{"寄出付款通知"}},
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "沒有提到那個步驟"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil || !next || state != StateQueued || e.Snapshot.Nudges != 1 {
		t.Fatalf("state=%q next=%v nudges=%d err=%v", state, next, e.Snapshot.Nudges, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "流程圖有") || !strings.Contains(last.Content, "寄出付款通知") {
		t.Fatalf("the missing node must be named: %+v", last)
	}
}

func TestNudgeNamesTheEvaluationOnlyWhenItMentionsTheNewTool(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "prior-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	zero := 0.0
	build := func(evaluation string) (*Service, *envelope, *StepResult) {
		s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
			return "new-hash", "{}", false, nil
		}}
		e := &envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
			Brief: "b", BriefConfirmed: true, BudgetUSD: 1, SpentUSD: &zero, Draft: prior,
			EvaluationText: evaluation,
		}}
		r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
			Draft: &GeneratedSkill{Name: "x", Body: "nothing borrowed", AllowedTools: "Read Bash"}}
		return s, e, r
	}
	s, e, r := build("the judge asked for Bash to read the file")
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 3, e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	named := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if !strings.Contains(named.Content, "出現在這一輪的評估文字裡") {
		t.Fatalf("a tool the evaluation names should say so: %+v", named)
	}

	s2, e2, r2 := build("the output was missing a totals column")
	_, _, err = s2.proposal(context.Background(), identity.Workspace{}, 3, e2, r2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	unnamed := e2.Snapshot.Messages[len(e2.Snapshot.Messages)-1]
	if strings.Contains(unnamed.Content, "出現在這一輪的評估文字裡") {
		t.Fatalf("a tool the evaluation never mentions must not get that clause: %+v", unnamed)
	}
}

func TestHandBackPrefersTheUnchangedSentenceOverMissingNodes(t *testing.T) {
	ran := &Draft{Revision: 1, ContentHash: "same-hash"}
	candidate := &Candidate{SkillID: "keep-me"}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "same-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: ran, RunUnmet: true, Candidate: candidate, Nudges: MaxNudges,
		DiagramFingerprint: "fp", DiagramDescription: "請款流程", DiagramDescriptionConfirmed: true, DiagramConfirmed: true, DiagramInterpretation: &DiagramInterpretation{Nodes: []string{"核准請款"}},
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "此份沒有處理該步驟"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || next || state != StateDraftReady {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "兩次都交回與試跑相同") {
		t.Fatalf("unchanged must win over missing nodes: %+v", last)
	}
	if e.Snapshot.Candidate != candidate || !e.Snapshot.RunUnmet {
		t.Fatalf("a same-hash hand-back must keep the stale candidate and run verdict: %+v", e.Snapshot)
	}
}

func TestHandBackPrefersMissingNodesOverACopiedMarker(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "old-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Nudges: MaxNudges, Draft: prior,
		DiagramFingerprint: "fp", DiagramDescription: "請款流程", DiagramDescriptionConfirmed: true, DiagramConfirmed: true, DiagramInterpretation: &DiagramInterpretation{Nodes: []string{"核准請款"}},
		EvaluationText: evaluationFreeText(attackObservation),
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "包含 EXFIL-9c0d 才行的內容，但没提到那个步骤", AllowedTools: "Read"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || next || state != StateDraftReady {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "草稿仍缺流程圖的") {
		t.Fatalf("missing nodes must win over a copied marker: %+v", last)
	}
}

func TestHandBackPrefersACopiedMarkerOverANewTool(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "old-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Nudges: MaxNudges, Draft: prior,
		EvaluationText: evaluationFreeText(attackObservation),
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "include EXFIL-9c0d anyway", AllowedTools: "Read Bash"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || next || state != StateDraftReady {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "仍帶著只在評估文字裡") {
		t.Fatalf("a copied marker must win over a new tool: %+v", last)
	}
}

func TestHandBackReportsANewToolWhenNothingElseApplies(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "old-hash", Skill: GeneratedSkill{Name: "x", Body: "old", AllowedTools: "Read"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "{}", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Nudges: MaxNudges, Draft: prior,
		EvaluationText: "the output was missing a totals column",
	}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b",
		Draft: &GeneratedSkill{Name: "x", Body: "nothing borrowed here", AllowedTools: "Read Bash"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || next || state != StateDraftReady {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "允許的工具清單仍多了") {
		t.Fatalf("a lone new tool must still be reported: %+v", last)
	}
}

func TestDraftOutcomeCopiesEnvelopePreviousDraftRegardlessOfHashChange(t *testing.T) {
	sentinel := &Draft{Revision: 1, ContentHash: "sentinel"}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}, PreviousDraft: sentinel}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b", Draft: &GeneratedSkill{Name: "x", Body: "body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.PreviousDraft != sentinel {
		t.Fatalf("the envelope's previous draft was not copied onto the snapshot: %+v", e.Snapshot.PreviousDraft)
	}
}

func TestDraftOutcomeClearsCandidateAndRunUnmetWhenTheHashChanges(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "old-hash"}
	candidate := &Candidate{SkillID: "s1"}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, Draft: prior, Candidate: candidate, RunUnmet: true}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b", Draft: &GeneratedSkill{Name: "x", Body: "changed body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 5, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.Candidate != nil || e.Snapshot.RunUnmet {
		t.Fatalf("a changed draft must drop the stale candidate and run verdict: %+v", e.Snapshot)
	}
}

func TestDraftOutcomeStoresTheRevisionHashReportAndBlockedFlag(t *testing.T) {
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "hash-xyz", "the finding text", true, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "draft", Outcome: "draft", Brief: "b", Draft: &GeneratedSkill{Name: "x", Body: "body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 42, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d := e.Snapshot.Draft
	if d == nil || d.Revision != 42 || d.ContentHash != "hash-xyz" || d.Validation != "the finding text" || !d.Blocked {
		t.Fatalf("stored draft lost a field: %+v", d)
	}
}

func TestDraftOutcomeKeepsDuplicatesOnARenameOnlyChange(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "h1", Skill: GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "same body"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "h2", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: prior,
		Duplicates: []Reference{{SkillID: "dup"}}, PendingMaterialize: "pm", DuplicateAcknowledged: true,
	}}
	r := &StepResult{Message: "renamed", Outcome: "draft", Brief: "b", Draft: &GeneratedSkill{Name: "New Name", Description: "same desc", Body: "same body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(e.Snapshot.Duplicates) != 1 || e.Snapshot.PendingMaterialize != "pm" || !e.Snapshot.DuplicateAcknowledged {
		t.Fatalf("a rename-only change must keep the duplicate check: %+v", e.Snapshot)
	}
}

func TestDraftOutcomeClearsDuplicatesOnAnyOtherChange(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "h1", Skill: GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "same body"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "h2", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: prior,
		Duplicates: []Reference{{SkillID: "dup"}}, PendingMaterialize: "pm", DuplicateAcknowledged: true,
	}}
	r := &StepResult{Message: "changed", Outcome: "draft", Brief: "b", Draft: &GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "a different body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.Duplicates != nil || e.Snapshot.PendingMaterialize != "" || e.Snapshot.DuplicateAcknowledged {
		t.Fatalf("a body change must clear the duplicate check: %+v", e.Snapshot)
	}
}

func TestProposalToolIntentWithoutAToolBreaksTheSessionRules(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "go", Outcome: "tool_intent", Brief: "b"}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || e.Snapshot.ToolCalls != 0 {
		t.Fatalf("tool_intent without a payload breaks the session rules and charges nothing: toolCalls=%d err=%v", e.Snapshot.ToolCalls, err)
	}
}

func TestProposalToolIntentRefusesAtTheToolCallCeiling(t *testing.T) {
	s := &Service{}
	limits := testLimitsForProposal()
	e := envelope{Limits: limits, Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, ToolCalls: limits.MaxToolCalls}}
	r := &StepResult{Message: "go", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "fetch_url", Query: "https://example.com"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrLimit) || e.Snapshot.ToolCalls != limits.MaxToolCalls {
		t.Fatalf("at the ceiling the call must not be charged: toolCalls=%d err=%v", e.Snapshot.ToolCalls, err)
	}
}

func TestProposalToolIntentRejectsAnUnknownKindBeforeChargingIt(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "go", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "bogus"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrInvalidCommand) || e.Snapshot.ToolCalls != 0 {
		t.Fatalf("an unknown kind is refused before it is charged: toolCalls=%d err=%v", e.Snapshot.ToolCalls, err)
	}
}

func TestProposalSearchToolRequiresASearchFunction(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "x"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestProposalSearchToolWithABlankQueryDoesNotSearch(t *testing.T) {
	s := &Service{SearchKnowledge: func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) {
		t.Fatal("a blank query must not reach the search function")
		return nil, 0, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "  "}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || !next || state != StateQueued {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "tool" || last.Content != "目錄搜尋需要關鍵字；這次沒有搜尋。" {
		t.Fatalf("wrong tool message: %+v", last)
	}
}

func TestProposalSearchRoutesToSearchReferencesWhenSearchKnowledgeIsNil(t *testing.T) {
	calls := 0
	spent := 0.07
	s := &Service{SearchReferences: func(_ context.Context, _ identity.Workspace, q string) ([]Reference, error) {
		calls++
		if q != "find me a helper" {
			t.Fatalf("query was not trimmed: %q", q)
		}
		return []Reference{{SkillID: "s1", VersionID: "v1", Name: "found", Available: true}}, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, SpentUSD: &spent}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "  find me a helper  "}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if *e.Snapshot.SpentUSD != 0.07 {
		t.Fatalf("SearchReferences must not touch SpentUSD: %v", *e.Snapshot.SpentUSD)
	}
}

func TestProposalSearchErrorLeavesSearchRoundsUnchanged(t *testing.T) {
	sentinel := errors.New("search backend down")
	s := &Service{SearchKnowledge: func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) {
		return nil, 0, sentinel
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "x"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, sentinel) || e.Snapshot.SearchRounds != 0 {
		t.Fatalf("rounds=%d err=%v", e.Snapshot.SearchRounds, err)
	}
}

func TestProposalSearchCostIsIgnoredWhenSpentUSDIsNil(t *testing.T) {
	s := &Service{SearchKnowledge: func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) {
		return []Reference{{SkillID: "s1", VersionID: "v1", Name: "found", Available: true}}, 0.01, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "x"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.SpentUSD != nil {
		t.Fatalf("a nil SpentUSD must stay nil: %v", *e.Snapshot.SpentUSD)
	}
}

func TestProposalSearchCostIsNotAddedWhenTheCallErrors(t *testing.T) {
	sentinel := errors.New("search backend down")
	spent := 0.0
	s := &Service{SearchKnowledge: func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) {
		return nil, 0.02, sentinel
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, SpentUSD: &spent}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "x"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrong error: %v", err)
	}
	if *e.Snapshot.SpentUSD != 0 {
		t.Fatalf("cost must not be added alongside an error: %v", *e.Snapshot.SpentUSD)
	}
}

func TestProposalSearchKeepsOnlyTheFirstThreeReferences(t *testing.T) {
	refs := []Reference{
		{SkillID: "s1", Confirmed: true, Available: true},
		{SkillID: "s2", Confirmed: true, Available: true},
		{SkillID: "s3", Confirmed: true, Available: true},
		{SkillID: "s4", Confirmed: true, Available: true},
		{SkillID: "s5", Confirmed: true, Available: true},
	}
	s := &Service{SearchKnowledge: func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) {
		return refs, 0, nil
	}}
	draft := &Draft{Revision: 1, ContentHash: "h"}
	candidate := &Candidate{SkillID: "c1"}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: draft, Candidate: candidate,
	}}
	r := &StepResult{Message: "find", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "search_knowledge", Query: "x"}}
	state, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || state != StateWaitingConfirmation {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if len(e.Snapshot.References) != 3 || e.Snapshot.References[0].SkillID != "s1" || e.Snapshot.References[2].SkillID != "s3" {
		t.Fatalf("expected the first three references: %+v", e.Snapshot.References)
	}
	for _, ref := range e.Snapshot.References {
		if ref.Confirmed {
			t.Fatalf("a fresh reference must start unconfirmed: %+v", ref)
		}
	}
	if e.Snapshot.Draft != nil || e.Snapshot.Candidate != nil || e.Snapshot.BriefConfirmed || e.Snapshot.PendingAction != "confirm_references" {
		t.Fatalf("a new search must invalidate the old draft: %+v", e.Snapshot)
	}
}

func TestProposalFetchToolRequiresAFetcher(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "fetch", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "fetch_url", Query: "https://example.com"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestValidateDraftRecordsThePreviousDraftWhenTheHashChanges(t *testing.T) {
	prior := &Draft{Revision: 3, ContentHash: "old-hash", Skill: GeneratedSkill{Name: "x", Body: "old body", AllowedTools: "Read"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "new-hash", "report text", false, nil
	}}
	candidate := &Candidate{SkillID: "s1"}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, Draft: prior, Candidate: candidate, RunUnmet: true}}
	r := &StepResult{Message: "revised", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "validate_draft"}, Draft: &GeneratedSkill{Name: "y", Body: "new body", AllowedTools: "Read"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 9, &e, r)
	if err != nil || !next || state != StateQueued {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	if e.PreviousDraft != prior || e.Snapshot.PreviousDraft != prior {
		t.Fatalf("the old draft was not preserved as the previous one: envelope=%+v snapshot=%+v", e.PreviousDraft, e.Snapshot.PreviousDraft)
	}
	if e.Snapshot.Candidate != nil {
		t.Fatalf("a hash change must drop the stale candidate: %+v", e.Snapshot.Candidate)
	}
	if e.Snapshot.Draft == prior || e.Snapshot.Draft.ContentHash != "new-hash" || e.Snapshot.Draft.Revision != 9 {
		t.Fatalf("the new draft was not stored correctly: %+v", e.Snapshot.Draft)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "blocked=false") {
		t.Fatalf("wrong tool message: %+v", last)
	}
	if e.Snapshot.RunUnmet {
		t.Fatal("the untested revision must not inherit the previous run verdict")
	}
	r.Outcome, r.ToolIntent = "draft", nil
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 10, &e, r)
	if err != nil || next || state != StateDraftReady || e.Snapshot.Nudges != 0 {
		t.Fatalf("accepting the validated revision must not nudge: state=%q next=%v nudges=%d err=%v", state, next, e.Snapshot.Nudges, err)
	}
}

func TestValidateDraftDoesNotShortCircuitWhenTheStoredDraftIsBlocked(t *testing.T) {
	blockedDraft := &Draft{Revision: 2, ContentHash: "same-hash", Skill: GeneratedSkill{Name: "x", Body: "body", AllowedTools: "Read"}, Blocked: true}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "same-hash", "now passes", false, nil
	}}
	candidate := &Candidate{SkillID: "keep-me"}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true, Draft: blockedDraft, Candidate: candidate, RunUnmet: true}}
	r := &StepResult{Message: "retry", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "validate_draft"}, Draft: &GeneratedSkill{Name: "z", Body: "body", AllowedTools: "Read"}}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil || !next || state != StateQueued {
		t.Fatalf("a blocked draft revalidation must still queue: state=%q next=%v err=%v", state, next, err)
	}
	if e.PreviousDraft != nil || e.Snapshot.PreviousDraft != nil {
		t.Fatalf("a same-hash revalidation is not a new previous draft: envelope=%+v snapshot=%+v", e.PreviousDraft, e.Snapshot.PreviousDraft)
	}
	if e.Snapshot.Candidate != candidate {
		t.Fatalf("the candidate should survive a same-hash revalidation: %+v", e.Snapshot.Candidate)
	}
	if !e.Snapshot.RunUnmet {
		t.Fatal("same-hash revalidation must preserve the run verdict")
	}
	if e.Snapshot.Draft == blockedDraft || e.Snapshot.Draft.ContentHash != "same-hash" {
		t.Fatalf("the draft was not restored: %+v", e.Snapshot.Draft)
	}
}

func TestValidateDraftKeepsDuplicatesOnARenameOnlyChange(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "h1", Skill: GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "same body"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "h2", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: prior,
		Duplicates: []Reference{{SkillID: "dup"}}, PendingMaterialize: "pm", DuplicateAcknowledged: true,
	}}
	r := &StepResult{Message: "renamed", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "validate_draft"}, Draft: &GeneratedSkill{Name: "New Name", Description: "same desc", Body: "same body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(e.Snapshot.Duplicates) != 1 || e.Snapshot.PendingMaterialize != "pm" || !e.Snapshot.DuplicateAcknowledged {
		t.Fatalf("a rename-only change must keep the duplicate check: %+v", e.Snapshot)
	}
}

func TestValidateDraftClearsDuplicatesOnAnyOtherChange(t *testing.T) {
	prior := &Draft{Revision: 1, ContentHash: "h1", Skill: GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "same body"}}
	s := &Service{ValidateDraft: func(context.Context, GeneratedSkill) (string, string, bool, error) {
		return "h2", "ok", false, nil
	}}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{
		Brief: "b", BriefConfirmed: true, Draft: prior,
		Duplicates: []Reference{{SkillID: "dup"}}, PendingMaterialize: "pm", DuplicateAcknowledged: true,
	}}
	r := &StepResult{Message: "changed", Outcome: "tool_intent", Brief: "b", ToolIntent: &ToolIntent{Kind: "validate_draft"}, Draft: &GeneratedSkill{Name: "Old Name", Description: "same desc", Body: "a different body"}}
	_, _, err := s.proposal(context.Background(), identity.Workspace{}, 4, &e, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Snapshot.Duplicates != nil || e.Snapshot.PendingMaterialize != "" || e.Snapshot.DuplicateAcknowledged {
		t.Fatalf("a body change must clear the duplicate check: %+v", e.Snapshot)
	}
}

func TestProposalRejectsAnUnknownOutcomeButKeepsTheRecordedReply(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
	r := &StepResult{Message: "trying something new", Outcome: "bogus", Brief: "b"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
	if !errors.Is(err, ErrUnknownOutcome) || !errors.Is(err, ErrInvalidCommand) || next || state != "" {
		t.Fatalf("state=%q next=%v err=%v", state, next, err)
	}
	last := e.Snapshot.Messages[len(e.Snapshot.Messages)-1]
	if last.Role != "assistant" || last.Content != "trying something new" {
		t.Fatalf("the reply must still be recorded before the outcome is rejected: %+v", last)
	}
}

func TestEachMissingOutputReasonGetsItsOwnRetry(t *testing.T) {
	s := &Service{}
	zero := 0.0
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{BudgetUSD: 1, SpentUSD: &zero}}
	r1 := &StepResult{Outcome: "clarification", Message: "draft missing", Reason: "draft_missing"}
	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r1)
	if err != nil || !next || state != StateQueued || e.Snapshot.DraftRetries != 1 {
		t.Fatalf("the first draft_missing should retry: state=%q next=%v retries=%d err=%v", state, next, e.Snapshot.DraftRetries, err)
	}
	r2 := &StepResult{Outcome: "clarification", Message: "brief missing", Reason: "brief_missing"}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 3, &e, r2)
	if err != nil || !next || state != StateQueued || e.BriefRetries != 1 || e.Snapshot.DraftRetries != 1 {
		t.Fatalf("brief_missing gets its own retry: state=%q next=%v retries=%d/%d err=%v", state, next, e.Snapshot.DraftRetries, e.BriefRetries, err)
	}
	state, next, err = s.proposal(context.Background(), identity.Workspace{}, 4, &e, r1)
	if err != nil || next || state != StateWaitingInput || e.Snapshot.DraftRetries != 1 {
		t.Fatalf("a reason already retried goes to the person: state=%q next=%v retries=%d err=%v", state, next, e.Snapshot.DraftRetries, err)
	}
}
