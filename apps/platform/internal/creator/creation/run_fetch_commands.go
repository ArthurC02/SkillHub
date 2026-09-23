package creation

import (
	"context"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func (s *Service) attachRun(ctx context.Context, ws identity.Workspace, p *Snapshot, runID string) (commandOutcome, error) {
	if p.Candidate == nil || s.ReadRun == nil || runID == "" || !p.hasRoomFor(1) {
		return commandOutcome{}, ErrInvalidCommand
	}
	observation, err := s.ReadRun(ctx, ws, runID, *p.Candidate)
	if err != nil {
		return commandOutcome{}, ErrNotFound
	}
	p.Candidate.RunID = runID
	p.RunUnmet = runUnmet(observation)
	observation = s.masked(observation)
	p.EvaluationText = evaluationFreeText(observation)
	p.appendMessage("tool", observation)
	if questions := trialQuestions(observation); p.RunUnmet && questions != "" {
		p.appendMessage("assistant", questions)
		p.PendingAction = NothingPending
		return settledIn(StateWaitingInput), nil
	}
	return stepQueued(), nil
}

func awaitsFetchConfirmation(p *Snapshot) bool {
	return p.PendingAction == PendingFetchPermission && p.PendingFetchURL != ""
}

func confirmFetch(p *Snapshot) (commandOutcome, error) {
	if !awaitsFetchConfirmation(p) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func declineFetch(p *Snapshot) (commandOutcome, error) {
	if !awaitsFetchConfirmation(p) {
		return commandOutcome{}, ErrInvalidCommand
	}
	rec := Fetch{URL: p.PendingFetchURL, Status: "declined"}
	p.Fetches = append(p.Fetches, rec)
	p.appendMessage("tool", fetchObservation(rec, ""))
	p.PendingFetchURL = ""
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func raiseBudget(p *Snapshot, l Limits, current State, budget float64) (commandOutcome, error) {
	if !finite(budget) || budget <= p.BudgetUSD || budget > l.MaxCostUSD {
		return commandOutcome{}, ErrBudgetOutOfBand
	}
	p.BudgetUSD = budget
	if current == StateFailed {
		return settledIn(StateWaitingInput), nil
	}
	return settledIn(current), nil
}
