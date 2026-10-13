package checks

import (
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
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

type Plan struct {
	decided      []verdict.CriterionResult
	plain        []testlab.Criterion
	pendingTrees []testlab.Criterion
	asks         judgeItems
}

func (p Plan) NeedsJudge() bool { return len(p.asks.list) > 0 }

func (p Plan) Decided() []verdict.CriterionResult { return p.decided }

func PlanCriteria(m facts.Material) Plan {
	var p Plan
	for _, c := range m.Criteria {
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

func treeResult(m facts.Material, c testlab.Criterion, verdicts map[string]verdict.CriterionResult) (verdict.CriterionResult, bool) {
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

func (p Plan) Finish(m facts.Material, judged []verdict.CriterionResult) []verdict.CriterionResult {
	verdicts := make(map[string]verdict.CriterionResult, len(judged))
	for _, r := range judged {
		verdicts[r.CriterionID] = r
	}
	out := append([]verdict.CriterionResult{}, p.decided...)
	for _, c := range p.plain {
		r, answered := verdicts[c.ID]
		if !answered {
			r = NoVerdict(c)
		}
		out = append(out, r)
	}
	for _, c := range p.pendingTrees {
		r, decided := treeResult(m, c, verdicts)
		if !decided {
			r = NoVerdict(c)
		}
		out = append(out, r)
	}
	return InCriteriaOrder(m.Criteria, out)
}

func NoVerdict(c testlab.Criterion) verdict.CriterionResult {
	return verdict.CriterionResult{
		CriterionID: c.ID, Text: c.Text, Result: verdict.ResultUndetermined, Source: verdict.SourceModel,
		Evidence: []verdict.EvidenceRef{}, Reason: "the judge returned no verdict for this criterion",
	}
}

func (p Plan) MaterialForJudge(m facts.Material) facts.Material {
	isCriterion := make(map[string]bool, len(m.Criteria))
	for _, c := range m.Criteria {
		isCriterion[c.ID] = true
	}
	m.Criteria = make([]testlab.Criterion, len(p.asks.list))
	m.Instructions = map[string]string{}
	for i, it := range p.asks.list {
		m.Criteria[i] = testlab.Criterion{ID: it.key, Text: it.text}
		if it.instructions != "" {
			m.Instructions[it.key] = it.instructions
		}
	}
	if m.Rubric == nil {
		return m
	}
	rubric := *m.Rubric
	rubric.Items = nil
	for _, it := range m.Rubric.Items {
		if p.asks.seen[it.ID] || !isCriterion[it.ID] {
			rubric.Items = append(rubric.Items, it)
		}
	}
	m.Rubric = &rubric
	return m
}
