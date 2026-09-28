package creation

import (
	"context"
	"errors"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestADiagramInterpretationReplyAsksThePersonToConfirmIt(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{DiagramFingerprint: "fp", DiagramDescriptionConfirmed: true}}
	r := &StepResult{Message: "m", Outcome: "confirm_diagram_interpretation", DiagramInterpretation: &DiagramDecomposition{Nodes: []string{"receive", "send"}}}

	state, next, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r)
	if err != nil || next || state != StateWaitingConfirmation || e.Snapshot.PendingAction != PendingDiagramInterpretation {
		t.Fatalf("state=%q next=%v pending=%q err=%v", state, next, e.Snapshot.PendingAction, err)
	}
}

func TestADiagramInterpretationUnderAnotherOutcomeIsRefused(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{DiagramFingerprint: "fp", DiagramDescriptionConfirmed: true}}
	r := &StepResult{Message: "m", Outcome: "clarification", DiagramInterpretation: &DiagramDecomposition{Nodes: []string{"receive"}}}

	if _, _, err := s.proposal(context.Background(), identity.Workspace{}, 2, &e, r); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("err = %v, want ErrInvalidCommand", err)
	}
}
