package checks

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
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

func ruledMaterial(final string, questions []trace.Question, status string, complete bool) facts.Material {
	m := facts.Material{
		Advanced: trace.AdvancedView{Complete: complete},
		Summary:  trace.Summary{FinalOutput: final, Questions: questions},
	}
	m.Run.Status, m.Run.Succeeded = status, status == "succeeded"
	return m
}

func toolMaterial(calls map[string]int, complete bool) facts.Material {
	m := ruledMaterial("答完了", nil, "succeeded", complete)
	m.Summary.ToolCalls.ByName = calls
	return m
}

func criterion(id, kind string, values ...string) testlab.Criterion {
	return testlab.Criterion{ID: id, Text: id, Check: &testlab.Check{Kind: kind, Values: values}}
}

var askedAboutChildren = []trace.Question{{Question: "小孩算不算人數？", Why: "會改變訂金", Options: []string{"算", "不算"}}}

func TestEachRuleCheckDecidesFromTheRunsOwnRecords(t *testing.T) {
	cases := []struct {
		name string
		m    facts.Material
		c    testlab.Criterion
		want string
	}{
		{"every value is in the answer", ruledMaterial("訂金 1,200 元，10月3日出發", nil, "succeeded", true),
			criterion("c", "contains_values", "1200", "2026-10-03"), verdict.ResultPassed},
		{"one value is missing from the answer", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "contains_values", "1200", "2026-10-03"), verdict.ResultFailed},
		{"the run asked", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks"), verdict.ResultPassed},
		{"the run asked about the named fact", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks", "小孩"), verdict.ResultPassed},
		{"the run asked about something else", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "asks", "預算"), verdict.ResultFailed},
		{"the run did not ask", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "asks"), verdict.ResultFailed},
		{"the run answered without asking", ruledMaterial("訂金 1,200 元", nil, "succeeded", true),
			criterion("c", "does_not_ask"), verdict.ResultPassed},
		{"the run asked when it should have answered", ruledMaterial("", askedAboutChildren, "succeeded", true),
			criterion("c", "does_not_ask"), verdict.ResultFailed},
		{"a failed run did not answer either", ruledMaterial("", nil, "failed", true),
			criterion("c", "does_not_ask"), verdict.ResultFailed},
		{"a pass on incomplete evidence is not recorded as passed", ruledMaterial("訂金 1,200 元", nil, "succeeded", false),
			criterion("c", "contains_values", "1200"), verdict.ResultUndetermined},
		{"an answer exactly at the character limit", ruledMaterial("一二三", nil, "succeeded", true),
			criterion("c", "max_chars", "3"), verdict.ResultPassed},
		{"an answer one character over the limit", ruledMaterial("一二三", nil, "succeeded", true),
			criterion("c", "max_chars", "2"), verdict.ResultFailed},
		{"whitespace does not count toward the character limit", ruledMaterial(" 一 二\n三 ", nil, "succeeded", true),
			criterion("c", "max_chars", "3"), verdict.ResultPassed},
		{"a failed run does not meet a character limit", ruledMaterial("", nil, "failed", true),
			criterion("c", "max_chars", "100"), verdict.ResultFailed},
		{"a failed run with a short partial output does not meet a character limit", ruledMaterial("一二", nil, "failed", true),
			criterion("c", "max_chars", "100"), verdict.ResultFailed},
		{"a run with no final output does not meet a character limit", ruledMaterial("  ", askedAboutChildren, "succeeded", true),
			criterion("c", "max_chars", "100"), verdict.ResultFailed},
		{"a stored limit that is not a number is undetermined", ruledMaterial("一二三", nil, "succeeded", true),
			criterion("c", "max_chars", "三"), verdict.ResultUndetermined},
		{"an answer exactly at the sentence limit", ruledMaterial("第一句。第二句！", nil, "succeeded", true),
			criterion("c", "max_sentences", "2"), verdict.ResultPassed},
		{"an answer one sentence over the limit", ruledMaterial("第一句。第二句！", nil, "succeeded", true),
			criterion("c", "max_sentences", "1"), verdict.ResultFailed},
		{"a line break also ends a sentence", ruledMaterial("第一行\n第二行", nil, "succeeded", true),
			criterion("c", "max_sentences", "1"), verdict.ResultFailed},
		{"a pass on a sentence limit with incomplete evidence is not recorded as passed", ruledMaterial("一句。", nil, "succeeded", false),
			criterion("c", "max_sentences", "1"), verdict.ResultUndetermined},
		{"the run called the named tool", toolMaterial(map[string]int{"WebSearch": 1}, true),
			criterion("c", "calls_tool", "websearch"), verdict.ResultPassed},
		{"the run called only one of two named tools", toolMaterial(map[string]int{"WebSearch": 2}, true),
			criterion("c", "calls_tool", "WebSearch", "Bash"), verdict.ResultFailed},
		{"the run called no tool when one was required", toolMaterial(nil, true),
			criterion("c", "calls_tool", "Bash"), verdict.ResultFailed},
		{"the run avoided the named tool", toolMaterial(map[string]int{"Read": 3}, true),
			criterion("c", "avoids_tool", "WebSearch"), verdict.ResultPassed},
		{"the run called a tool it should have avoided", toolMaterial(map[string]int{"Read": 3, "websearch": 1}, true),
			criterion("c", "avoids_tool", "WebSearch"), verdict.ResultFailed},
		{"avoiding a tool on incomplete evidence is not recorded as passed", toolMaterial(nil, false),
			criterion("c", "avoids_tool", "WebSearch"), verdict.ResultUndetermined},
		{"a fail on incomplete evidence stays failed", ruledMaterial("訂金 1,200 元", nil, "succeeded", false),
			criterion("c", "contains_values", "999"), verdict.ResultFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, decided := treeResult(c.m, c.c, nil)
			if !decided || got.Result != c.want || got.Source != verdict.SourceRule || got.Reason == "" {
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
	m.Criteria = criteria
	p := PlanCriteria(m)
	if len(p.decided) != 1 || p.decided[0].CriterionID != "ruled" {
		t.Errorf("decided = %+v", p.decided)
	}
	if len(p.asks.list) != 2 || p.asks.list[0].key != "plain" || p.asks.list[1].key != "gated" {
		t.Errorf("to judge = %+v", p.asks.list)
	}
	if len(p.pendingTrees) != 1 || p.pendingTrees[0].ID != "gated" {
		t.Errorf("pending trees = %+v", p.pendingTrees)
	}
}

func TestResultsComeBackInTheSnapshotsCriterionOrder(t *testing.T) {
	criteria := []testlab.Criterion{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := InCriteriaOrder(criteria, []verdict.CriterionResult{
		{CriterionID: "c", Source: verdict.SourceRule}, {CriterionID: "a", Source: verdict.SourceRule}, {CriterionID: "b", Source: verdict.SourceModel},
	})
	if len(got) != 3 || got[0].CriterionID != "a" || got[1].CriterionID != "b" || got[2].CriterionID != "c" {
		t.Errorf("order = %+v", got)
	}
}

func TestTheJudgeLosesOnlyTheRubricItemsOfRuledCriteria(t *testing.T) {
	m := facts.Material{
		Criteria: []testlab.Criterion{{ID: "plain"}, criterion("ruled", "does_not_ask")},
		Rubric: &testlab.Rubric{Version: "r1", Items: []testlab.RubricItem{
			{ID: "plain"}, {ID: "ruled"}, {ID: "orphan"},
		}},
	}
	judged := PlanCriteria(m).MaterialForJudge(m)
	if len(judged.Criteria) != 1 || judged.Criteria[0].ID != "plain" {
		t.Errorf("judged criteria = %+v", judged.Criteria)
	}
	if items := judged.Rubric.Items; len(items) != 2 || items[0].ID != "plain" || items[1].ID != "orphan" {
		t.Errorf("judged rubric = %+v, want plain and the orphan the judge path reports", items)
	}
	if len(m.Rubric.Items) != 3 {
		t.Errorf("the original rubric was changed: %+v", m.Rubric.Items)
	}
}

func TestQuestionChecksReadTheWholeConversation(t *testing.T) {
	answered := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	answered.EarlierQuestions = askedAboutChildren
	cases := []struct {
		name string
		c    testlab.Criterion
		want string
	}{
		{"an earlier round asked", criterion("c", "asks"), verdict.ResultPassed},
		{"an earlier round asked about the named fact", criterion("c", "asks", "小孩"), verdict.ResultPassed},
		{"an earlier round asked about something else", criterion("c", "asks", "預算"), verdict.ResultFailed},
		{"an earlier round asked when none should have", criterion("c", "does_not_ask"), verdict.ResultFailed},
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
		m    facts.Material
		want bool
	}{
		{"succeeded by asking", ruledMaterial("", askedAboutChildren, "succeeded", true), true},
		{"failed after asking", ruledMaterial("", askedAboutChildren, "failed", true), false},
		{"succeeded with an answer", ruledMaterial("訂金 1,200 元", nil, "succeeded", true), false},
	}
	for _, c := range cases {
		if got := c.m.AwaitsAnswer(); got != c.want {
			t.Errorf("%s: awaitsAnswer = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCountsFollowTheSkillsOwnCheckScript(t *testing.T) {
	cases := []struct {
		name, text       string
		chars, sentences int
	}{
		{"empty", "", 0, 0},
		{"whitespace only", " \n\t", 0, 0},
		{"one sentence with a full stop", "今天下雨。", 5, 1},
		{"mixed punctuation", "要帶傘嗎？要!好的。", 10, 3},
		{"break with no text after it", "一。\n\n二。", 4, 2},
		{"latin words keep their letters", "ab cd", 4, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countChars(c.text); got != c.chars {
				t.Errorf("countChars(%q) = %d, want %d", c.text, got, c.chars)
			}
			if got := countSentences(c.text); got != c.sentences {
				t.Errorf("countSentences(%q) = %d, want %d", c.text, got, c.sentences)
			}
		})
	}
}
