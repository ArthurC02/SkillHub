package eval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var PanelRoles = []string{"skillhub-judge", "skillhub-judge-panel-1", "skillhub-judge-panel-2"}

func (s *Service) panelRoles(ctx context.Context) ([]string, error) {
	if s.PanelEnabled == nil {
		return nil, nil
	}
	on, err := s.PanelEnabled(ctx)
	if err != nil || !on {
		return nil, err
	}
	return PanelRoles, nil
}

func (s *Service) judgeAll(ctx context.Context, req JudgeRequest) ([]*Judgement, error) {
	roles, err := s.panelRoles(ctx)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		resp, err := s.Judge.JudgeRun(ctx, req)
		return []*Judgement{resp}, err
	}
	answers := make([]*Judgement, len(roles))
	failures := make([]error, len(roles))
	var wg sync.WaitGroup
	for i, role := range roles {
		wg.Go(func() {
			member := req
			member.ModelRole = role
			answers[i], failures[i] = s.Judge.JudgeRun(ctx, member)
		})
	}
	wg.Wait()
	if err := errors.Join(failures...); err != nil {
		return nil, fmt.Errorf("a judge panel member did not answer, so the panel has no verdict: %w", err)
	}
	return answers, nil
}

func vote(members [][]CriterionResult) (results []CriterionResult, split []string) {
	results = make([]CriterionResult, len(members[0]))
	for i := range members[0] {
		counts := map[string]int{}
		for _, m := range members {
			counts[m[i].Result]++
		}
		winner, ok := majority(counts, len(members))
		if !ok {
			results[i] = undecided(members[0][i], counts)
			split = append(split, members[0][i].CriterionID)
			continue
		}
		for _, m := range members {
			if m[i].Result == winner {
				results[i] = m[i]
				break
			}
		}
	}
	return results, split
}

func majority(counts map[string]int, voters int) (string, bool) {
	for result, n := range counts {
		if n*2 > voters {
			return result, true
		}
	}
	return "", false
}

func undecided(first CriterionResult, counts map[string]int) CriterionResult {
	tally := make([]string, 0, len(counts))
	for _, result := range []string{ResultPassed, ResultFailed, ResultUndetermined} {
		if counts[result] > 0 {
			tally = append(tally, fmt.Sprintf("%s by %d", result, counts[result]))
		}
	}
	first.Result = ResultUndetermined
	first.Evidence = []EvidenceRef{}
	first.Reason = "the judges did not agree (" + strings.Join(tally, ", ") + "), so no result is recorded"
	return first
}

func panelModel(answers []*Judgement) string {
	models := make([]string, len(answers))
	for i, a := range answers {
		models[i] = orUnknown(a.Model)
	}
	return strings.Join(models, " + ")
}

func panelUsage(answers []*Judgement) *ModelUsage {
	if len(answers) == 1 {
		return answers[0].Usage
	}
	total := &ModelUsage{CostReported: true}
	var cost float64
	for _, a := range answers {
		u := a.Usage
		if u == nil {
			total.CostReported = false
			continue
		}
		total.PromptTokens += u.PromptTokens
		total.CompletionTokens += u.CompletionTokens
		if c := u.ReportedCostUSD(); c != nil {
			cost += *c
		} else {
			total.CostReported = false
		}
	}
	if total.CostReported {
		total.CostUSD = &cost
	}
	return total
}
