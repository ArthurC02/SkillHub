package eval

import (
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

type criteriaPlan struct {
	decided      []CriterionResult
	toJudge      []testlab.Criterion
	pendingTrees []testlab.Criterion
}

func planCriteria(m material) criteriaPlan {
	var p criteriaPlan
	for _, c := range m.criteria {
		if c.Check == nil {
			p.toJudge = append(p.toJudge, c)
			continue
		}
		if r, decided := treeResult(m, c, nil); decided {
			p.decided = append(p.decided, r)
			continue
		}
		p.toJudge = append(p.toJudge, c)
		p.pendingTrees = append(p.pendingTrees, c)
	}
	return p
}

func treeResult(m material, c testlab.Criterion, judge *CriterionResult) (CriterionResult, bool) {
	var steps []CheckStep
	o := evaluateCheck(checkScope{m: m, judge: judge, steps: &steps}, *c.Check, "0")
	if o.result == resultPending {
		return CriterionResult{}, false
	}
	source := SourceRule
	if o.judged {
		source = SourceModel
	}
	evidence := o.evidence
	if evidence == nil {
		evidence = []EvidenceRef{}
	}
	r := CriterionResult{CriterionID: c.ID, Text: c.Text, Result: o.result, Source: source, Evidence: evidence, Reason: o.reason}
	if len(steps) > 1 {
		r.Steps = steps
	}
	return r, true
}

func (p criteriaPlan) finish(m material, judged []CriterionResult) []CriterionResult {
	byID := make(map[string]CriterionResult, len(judged))
	for _, r := range judged {
		byID[r.CriterionID] = r
	}
	out := append([]CriterionResult{}, p.decided...)
	inTree := make(map[string]bool, len(p.pendingTrees))
	for _, c := range p.pendingTrees {
		inTree[c.ID] = true
		out = append(out, finishedTree(m, c, byID))
	}
	for _, r := range judged {
		if !inTree[r.CriterionID] {
			out = append(out, r)
		}
	}
	return inCriteriaOrder(m.criteria, out)
}

func finishedTree(m material, c testlab.Criterion, judged map[string]CriterionResult) CriterionResult {
	verdict, found := judged[c.ID]
	if found {
		if r, decided := treeResult(m, c, &verdict); decided {
			return r
		}
	}
	return CriterionResult{
		CriterionID: c.ID, Text: c.Text, Result: ResultUndetermined, Source: SourceModel,
		Evidence: []EvidenceRef{}, Reason: "the judge returned no verdict for this criterion",
	}
}
