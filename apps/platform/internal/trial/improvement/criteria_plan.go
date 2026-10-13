package eval

import (
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

type judgeItem struct {
	key          string
	text         string
	instructions string
}

type judgeItems struct {
	list []judgeItem
	seen map[string]bool
}

func (j *judgeItems) add(it judgeItem) {
	if j.seen[it.key] {
		return
	}
	if j.seen == nil {
		j.seen = map[string]bool{}
	}
	j.seen[it.key] = true
	j.list = append(j.list, it)
}

type criteriaPlan struct {
	decided      []verdict.CriterionResult
	plain        []testlab.Criterion
	pendingTrees []testlab.Criterion
	asks         judgeItems
}

func planCriteria(m material) criteriaPlan {
	var p criteriaPlan
	for _, c := range m.criteria {
		if c.Check == nil {
			p.plain = append(p.plain, c)
			p.asks.add(judgeItem{key: c.ID, text: c.Text})
			continue
		}
		if r, decided := treeResult(m, c, nil); decided {
			p.decided = append(p.decided, r)
			continue
		}
		p.pendingTrees = append(p.pendingTrees, c)
		evaluateCheck(checkScope{m: m, criterion: c, asking: &p.asks}, *c.Check, "0")
	}
	return p
}

func treeResult(m material, c testlab.Criterion, verdicts map[string]verdict.CriterionResult) (verdict.CriterionResult, bool) {
	var steps []verdict.CheckStep
	o := evaluateCheck(checkScope{m: m, criterion: c, verdicts: verdicts, steps: &steps}, *c.Check, "0")
	if o.result == resultPending {
		return verdict.CriterionResult{}, false
	}
	source := verdict.SourceRule
	if o.judged {
		source = verdict.SourceModel
	}
	evidence := o.evidence
	if evidence == nil {
		evidence = []verdict.EvidenceRef{}
	}
	r := verdict.CriterionResult{CriterionID: c.ID, Text: c.Text, Result: o.result, Source: source, Evidence: evidence, Reason: o.reason}
	if len(steps) > 1 {
		r.Steps = steps
	}
	return r, true
}

func (p criteriaPlan) finish(m material, judged []verdict.CriterionResult) []verdict.CriterionResult {
	verdicts := make(map[string]verdict.CriterionResult, len(judged))
	for _, r := range judged {
		verdicts[r.CriterionID] = r
	}
	out := append([]verdict.CriterionResult{}, p.decided...)
	for _, c := range p.plain {
		r, answered := verdicts[c.ID]
		if !answered {
			r = noVerdict(c)
		}
		out = append(out, r)
	}
	for _, c := range p.pendingTrees {
		r, decided := treeResult(m, c, verdicts)
		if !decided {
			r = noVerdict(c)
		}
		out = append(out, r)
	}
	return inCriteriaOrder(m.criteria, out)
}

func noVerdict(c testlab.Criterion) verdict.CriterionResult {
	return verdict.CriterionResult{
		CriterionID: c.ID, Text: c.Text, Result: verdict.ResultUndetermined, Source: verdict.SourceModel,
		Evidence: []verdict.EvidenceRef{}, Reason: "the judge returned no verdict for this criterion",
	}
}
