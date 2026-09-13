package creation

import "slices"

type State string

const (
	StateQueued              State = "queued"
	StateWorking             State = "working"
	StateWaitingInput        State = "waiting_input"
	StateWaitingConfirmation State = "waiting_confirmation"
	StateDraftReady          State = "draft_ready"
	StateCandidateReady      State = "candidate_ready"
	StateSaved               State = "saved"
	StateCancelled           State = "cancelled"
	StateFailed              State = "failed"
	StateNeedsReupload       State = "needs_reupload"
)

func AllStates() []State {
	return []State{
		StateQueued,
		StateWorking,
		StateWaitingInput,
		StateWaitingConfirmation,
		StateDraftReady,
		StateCandidateReady,
		StateSaved,
		StateCancelled,
		StateFailed,
		StateNeedsReupload,
	}
}

var successors = map[State][]State{
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

func ParseState(s string) (State, bool) {
	candidate := State(s)
	if slices.Contains(AllStates(), candidate) {
		return candidate, true
	}
	return "", false
}

func (s State) HasEnded() bool {
	return s == StateSaved || s == StateCancelled
}

func (s State) AwaitsTheModel() bool {
	return s == StateQueued || s == StateWorking
}

func CanTransition(from, to State) bool {
	if _, known := ParseState(string(from)); !known {
		return false
	}
	if _, known := ParseState(string(to)); !known {
		return false
	}
	if from == to {
		return true
	}
	return slices.Contains(successors[from], to)
}
