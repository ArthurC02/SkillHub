package eval

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func TestAValueIsMentionedInAnyOfItsCommonWrittenForms(t *testing.T) {
	cases := []struct {
		output, value string
		want          bool
	}{
		{"訂金共 1,200 元", "1200", true},
		{"訂金共 1200 元", "1,200", false},
		{"訂金共1200元", "1200", true},
		{"總共 120 元", "12", false},
		{"總共 3.12 元", "12", false},
		{"總共 12 元", "12", true},
		{"總共 12.5 元", "12", false},
		{"總共 12. 下一項", "12", true},
		{"總共 1,200.5 元", "1200", false},
		{"負 -30 元", "-30", true},
		{"出發日 2026/10/03", "2026-10-03", true},
		{"出發日 2026/10/3", "2026-10-03", true},
		{"出發日 2026年10月3日", "2026-10-03", true},
		{"出發日 10月3日", "2026-10-03", true},
		{"出發日 10/31", "2026-10-03", false},
		{"共 2小時30分", "2:30", true},
		{"共 2小時", "2:00", true},
		{"共 2小時", "2:30", false},
		{"請改用 PDF 檔", "PDF", true},
		{"請改用 PDF 檔", "XLSX", false},
	}
	for _, c := range cases {
		if got := mentions(c.output, c.value); got != c.want {
			t.Errorf("mentions(%q, %q) = %v, want %v", c.output, c.value, got, c.want)
		}
	}
}

func ruledMaterial(final string, questions []trace.Question, status string, complete bool) material {
	m := material{
		advanced: trace.AdvancedView{Complete: complete},
		summary:  trace.Summary{FinalOutput: final, Questions: questions},
	}
	m.run.Status = status
	return m
}

func criterion(id, kind string, values ...string) testlab.Criterion {
	return testlab.Criterion{ID: id, Text: id, Check: &testlab.Check{Kind: kind, Values: values}}
}

var askedAboutChildren = []trace.Question{{Question: "小孩算不算人數？", Why: "會改變訂金", Options: []string{"算", "不算"}}}

func TestEachRuleCheckDecidesFromTheRunsOwnRecords(t *testing.T) {
	cases := []struct {
		name string
		m    material
		c    testlab.Criterion
		want string
	}{
		{"every value is in the answer", ruledMaterial("訂金 1,200 元，10月3日出發", nil, "succeeded", true),
			criterion("c", "contains_values", "1200", "2026-10-03"), ResultPassed},
		{"one value is missing from the answer", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "contains_values", "1200", "2026-10-03"), ResultFailed},
		{"the run asked", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks"), ResultPassed},
		{"the run asked about the named fact", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks", "小孩"), ResultPassed},
		{"the run asked about something else", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks", "預算"), ResultFailed},
		{"the run did not ask", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "asks"), ResultFailed},
		{"the run answered without asking", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "does_not_ask"), ResultPassed},
		{"the run asked when it should have answered", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "does_not_ask"), ResultFailed},
		{"a failed run did not answer either", ruledMaterial("", nil, "failed", true),
			criterion("c", "does_not_ask"), ResultFailed},
		{"a pass on incomplete evidence is not recorded as passed", ruledMaterial("訂金 1,200 元", nil, "succeeded", false),
			criterion("c", "contains_values", "1200"), ResultUndetermined},
		{"a fail on incomplete evidence stays failed", ruledMaterial("訂金 1,200 元", nil, "succeeded", false),
			criterion("c", "contains_values", "999"), ResultFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, decided := treeResult(c.m, c.c, nil)
			if !decided || got.Result != c.want || got.Source != SourceRule || got.Reason == "" {
				t.Fatalf("got %+v (decided %v), want one %s result from the rule with a reason", got, decided, c.want)
			}
		})
	}
}

func TestOnlyCriteriaWhoseCheckNeedsNoJudgeSkipTheJudge(t *testing.T) {
	judgedAfterARule := testlab.Criterion{ID: "gated", Text: "gated", Check: &testlab.Check{
		Kind: "sequence", Children: []testlab.Check{{Kind: "does_not_ask"}, {Kind: "judge"}},
	}}
	criteria := []testlab.Criterion{{ID: "plain", Text: "reads well"}, criterion("ruled", "does_not_ask"), judgedAfterARule}
	m := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	m.criteria = criteria
	p := planCriteria(m)
	if len(p.decided) != 1 || p.decided[0].CriterionID != "ruled" {
		t.Errorf("decided = %+v", p.decided)
	}
	if len(p.toJudge) != 2 || p.toJudge[0].ID != "plain" || p.toJudge[1].ID != "gated" {
		t.Errorf("to judge = %+v", p.toJudge)
	}
	if len(p.pendingTrees) != 1 || p.pendingTrees[0].ID != "gated" {
		t.Errorf("pending trees = %+v", p.pendingTrees)
	}
}

func TestResultsComeBackInTheSnapshotsCriterionOrder(t *testing.T) {
	criteria := []testlab.Criterion{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := inCriteriaOrder(criteria, []CriterionResult{
		{CriterionID: "c", Source: SourceRule}, {CriterionID: "a", Source: SourceRule}, {CriterionID: "b", Source: SourceModel},
	})
	if len(got) != 3 || got[0].CriterionID != "a" || got[1].CriterionID != "b" || got[2].CriterionID != "c" {
		t.Errorf("order = %+v", got)
	}
}

func TestTheJudgeLosesOnlyTheRubricItemsOfRuledCriteria(t *testing.T) {
	m := material{
		criteria: []testlab.Criterion{{ID: "plain"}, criterion("ruled", "does_not_ask")},
		rubric: &testlab.Rubric{Version: "r1", Items: []testlab.RubricItem{
			{ID: "plain"}, {ID: "ruled"}, {ID: "orphan"},
		}},
	}
	judged := m.judging(planCriteria(m))
	if len(judged.criteria) != 1 || judged.criteria[0].ID != "plain" {
		t.Errorf("judged criteria = %+v", judged.criteria)
	}
	if items := judged.rubric.Items; len(items) != 2 || items[0].ID != "plain" || items[1].ID != "orphan" {
		t.Errorf("judged rubric = %+v, want plain and the orphan the judge path reports", items)
	}
	if len(m.rubric.Items) != 3 {
		t.Errorf("the original rubric was changed: %+v", m.rubric.Items)
	}
}

func TestQuestionChecksReadTheWholeConversation(t *testing.T) {
	answered := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	answered.earlierQuestions = askedAboutChildren
	cases := []struct {
		name string
		c    testlab.Criterion
		want string
	}{
		{"an earlier round asked", criterion("c", "asks"), ResultPassed},
		{"an earlier round asked about the named fact", criterion("c", "asks", "小孩"), ResultPassed},
		{"an earlier round asked about something else", criterion("c", "asks", "預算"), ResultFailed},
		{"an earlier round asked when none should have", criterion("c", "does_not_ask"), ResultFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := treeResult(answered, c.c, nil); got.Result != c.want {
				t.Fatalf("got %+v, want %s", got, c.want)
			}
		})
	}
}

func TestOnlyARunThatSucceededByAskingWaitsForItsAnswer(t *testing.T) {
	cases := []struct {
		name string
		m    material
		want bool
	}{
		{"succeeded by asking", ruledMaterial("", askedAboutChildren, "succeeded", true), true},
		{"failed after asking", ruledMaterial("", askedAboutChildren, "failed", true), false},
		{"succeeded with an answer", ruledMaterial("訂金 1,200 元", nil, "succeeded", true), false},
	}
	for _, c := range cases {
		if got := c.m.awaitsAnswer(); got != c.want {
			t.Errorf("%s: awaitsAnswer = %v, want %v", c.name, got, c.want)
		}
	}
}
