package creation

import (
	"slices"
	"testing"
)

var legalTransitions = map[State][]State{
	StateQueued: {
		StateWorking, StateWaitingInput,
		StateFailed, StateNeedsReupload, StateCancelled,
	},
	StateWorking: {
		StateQueued, StateWaitingInput, StateWaitingConfirmation, StateDraftReady,
		StateFailed, StateNeedsReupload, StateCancelled,
	},
	StateWaitingInput: {
		StateQueued, StateWaitingConfirmation, StateCandidateReady,
		StateSaved, StateCancelled,
	},
	StateWaitingConfirmation: {
		StateQueued, StateWaitingInput, StateCandidateReady,
		StateSaved, StateCancelled,
	},
	StateDraftReady: {
		StateQueued, StateWaitingInput, StateWaitingConfirmation, StateCandidateReady,
		StateSaved, StateCancelled,
	},
	StateCandidateReady: {
		StateQueued, StateWaitingInput, StateWaitingConfirmation,
		StateSaved, StateCancelled,
	},
	StateFailed: {
		StateQueued, StateWaitingInput, StateWaitingConfirmation, StateCandidateReady,
		StateSaved, StateCancelled,
	},
	StateNeedsReupload: {
		StateQueued, StateWaitingInput, StateWaitingConfirmation, StateCandidateReady,
		StateSaved, StateCancelled,
	},
}

func TestTheTableAdmitsExactlyTheTransitionsTheCodeCanProduce(t *testing.T) {
	if len(AllStates()) != 10 {
		t.Fatalf("the session vocabulary is ten states, got %d", len(AllStates()))
	}
	for _, from := range AllStates() {
		for _, to := range AllStates() {
			if from == to {
				continue
			}
			want := slices.Contains(legalTransitions[from], to)
			if got := CanTransition(from, to); got != want {
				t.Errorf("%s -> %s: table says %v, the traced code paths say %v", from, to, got, want)
			}
		}
	}
}

func TestASessionThatHasEndedHasNowhereToGo(t *testing.T) {
	for _, ended := range []State{StateSaved, StateCancelled} {
		if !ended.HasEnded() {
			t.Fatalf("%s must count as ended", ended)
		}
		for _, to := range AllStates() {
			if to == ended {
				continue
			}
			if CanTransition(ended, to) {
				t.Errorf("%s is a dead end, yet the table lets it reach %s", ended, to)
			}
		}
	}
}

func TestFailedIsRecoverableAndThereforeNotAnEnding(t *testing.T) {
	if StateFailed.HasEnded() {
		t.Fatal("a failed session can be revived by raising the budget, so it has not ended")
	}
	for _, to := range []State{StateWaitingInput, StateQueued, StateCancelled} {
		if !CanTransition(StateFailed, to) {
			t.Errorf("a failed session must still be able to reach %s", to)
		}
	}
}

func TestRewritingTheSameStateIsAlwaysAllowed(t *testing.T) {
	for _, s := range AllStates() {
		if !CanTransition(s, s) {
			t.Errorf("%s: settling an attempt rewrites the state unchanged, which must not be refused", s)
		}
	}
}

func TestOnlyTheModelProducesADraft(t *testing.T) {
	for _, from := range AllStates() {
		if from == StateWorking || from == StateDraftReady {
			continue
		}
		if CanTransition(from, StateDraftReady) {
			t.Errorf("a draft is the outcome of a model step, so %s must not reach %s", from, StateDraftReady)
		}
	}
	if !CanTransition(StateWorking, StateDraftReady) {
		t.Fatalf("%s is exactly where a draft comes from", StateWorking)
	}
}

func TestAStepStartsOnlyFromTheQueue(t *testing.T) {
	for _, from := range AllStates() {
		if from == StateQueued || from == StateWorking {
			continue
		}
		if CanTransition(from, StateWorking) {
			t.Errorf("%s must enqueue before the model runs, it cannot reach %s directly", from, StateWorking)
		}
	}
	if !CanTransition(StateQueued, StateWorking) {
		t.Fatal("a queued session must be able to start")
	}
}

func TestEveryUnfinishedSessionCanBeCancelled(t *testing.T) {
	for _, from := range AllStates() {
		if from.HasEnded() {
			continue
		}
		if !CanTransition(from, StateCancelled) {
			t.Errorf("cancel is offered in %s, so the table must admit it", from)
		}
	}
}

func TestOnlyTheQueueAndTheModelHoldTheTurn(t *testing.T) {
	for _, s := range AllStates() {
		want := s == StateQueued || s == StateWorking
		if s.AwaitsTheModel() != want {
			t.Errorf("%s: AwaitsTheModel is %v, want %v", s, s.AwaitsTheModel(), want)
		}
	}
}

func TestAStateNobodyDeclaredIsRefusedOnBothSides(t *testing.T) {
	for _, bogus := range []string{"", "done", "Queued", "waiting-input", "saved "} {
		if _, ok := ParseState(bogus); ok {
			t.Errorf("%q is not a session state", bogus)
		}
		if CanTransition(State(bogus), StateCancelled) {
			t.Errorf("a session sitting on %q must not be advanced", bogus)
		}
		if CanTransition(StateQueued, State(bogus)) {
			t.Errorf("%q must not be written into the state column", bogus)
		}
	}
	for _, known := range AllStates() {
		if _, ok := ParseState(string(known)); !ok {
			t.Errorf("%s is declared but ParseState rejects it", known)
		}
	}
}
