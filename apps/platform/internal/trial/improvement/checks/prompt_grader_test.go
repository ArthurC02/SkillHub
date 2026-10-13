package checks

import (
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

const toneV2 = "0b6f9c1e-0000-4000-8000-000000000002"

func graded(m facts.Material) facts.Material {
	m.Graders = map[string]graders.Version{toneV2: {Name: "語氣", Number: 2, Instructions: "fail when {item} sounds curt"}}
	return m
}

func keysAsked(t *testing.T, m facts.Material, check testlab.Check) []string {
	t.Helper()
	m.Criteria = []testlab.Criterion{{ID: "c", Text: "reads well", Check: &check}}
	var keys []string
	for _, it := range PlanCriteria(m).asks.list {
		keys = append(keys, it.key)
	}
	return keys
}

func TestOnlyTheModelChecksAWaitingTreeCouldStillReachAreAsked(t *testing.T) {
	m := graded(ruledMaterial("訂金 1,200 元", nil, "succeeded", true))
	grader := leaf(promptGraderKind, toneV2)
	cases := []struct {
		name  string
		check testlab.Check
		want  []string
	}{
		{"a sequence asks every model step after a waiting one", node("sequence", leaf("judge"), grader), []string{"c", "c@0.1"}},
		{"a failed rule ends the sequence before the steps behind it", node("sequence", leaf("judge"), leaf("contains_values", "999"), grader), []string{"c"}},
		{"a waiting condition asks both of its branches", node("if", leaf("judge"), grader, grader), []string{"c", "c@0.1", "c@0.2"}},
		{"a decided condition asks only its taken branch", node("if", leaf("does_not_ask"), grader, leaf("judge")), []string{"c@0.1"}},
		{"a rule gate that already failed asks nothing", node("sequence", leaf("contains_values", "999"), grader), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keysAsked(t, m, c.check); !slices.Equal(got, c.want) {
				t.Errorf("asked %v, want %v", got, c.want)
			}
		})
	}
}

func TestAGraderVersionTheWorkspaceCannotSeeIsUndeterminedWithoutAsking(t *testing.T) {
	m := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	if keys := keysAsked(t, m, leaf(promptGraderKind, toneV2)); keys != nil {
		t.Errorf("asked %v for a grader this workspace cannot see", keys)
	}
	got, decided := decide(t, m, leaf(promptGraderKind, toneV2), nil)
	if !decided || got.Result != verdict.ResultUndetermined || got.Source != verdict.SourceRule {
		t.Errorf("got %+v (decided %v), want undetermined without the judge", got, decided)
	}
}

func TestEachPromptGraderLeafGetsItsOwnInstructionsAndVerdict(t *testing.T) {
	m := graded(ruledMaterial("訂金 1,200 元", nil, "succeeded", true))
	fanOut := testlab.Check{Kind: "each", Values: []string{"the greeting", "the closing"}, AtLeast: 1,
		Children: []testlab.Check{leaf(promptGraderKind, toneV2)}}
	m.Criteria = []testlab.Criterion{{ID: "c", Text: "reads well", Check: &fanOut}}
	asks := PlanCriteria(m).asks.list
	if len(asks) != 2 || asks[0].key != "c@0.0" || asks[0].instructions != "fail when the greeting sounds curt" ||
		asks[1].key != "c@0.1" || asks[1].instructions != "fail when the closing sounds curt" {
		t.Fatalf("asked %+v, want one item per value with {item} filled in", asks)
	}

	got, decided := decide(t, m, fanOut, map[string]verdict.CriterionResult{
		"c@0.0": {Result: verdict.ResultFailed, Reason: "curt"},
		"c@0.1": {Result: verdict.ResultPassed, Reason: "warm"},
	})
	if !decided || got.Result != verdict.ResultPassed || got.Source != verdict.SourceModel {
		t.Fatalf("got %+v, want passed from the model with one of two items", got)
	}
	if !strings.HasPrefix(got.Steps[0].Reason, "語氣 v2: ") {
		t.Errorf("first step reason = %q, want it to name the grader and version", got.Steps[0].Reason)
	}
}

func TestTheJudgeIsSentEachAskedItemWithItsInstructionsAndOnlyTheRubricItemsItCanUse(t *testing.T) {
	gate := node("sequence", leaf("does_not_ask"), leaf(promptGraderKind, toneV2))
	m := graded(ruledMaterial("訂金 1,200 元", nil, "succeeded", true))
	m.Criteria = []testlab.Criterion{
		{ID: "plain", Text: "is correct"}, {ID: "graded", Text: "reads well", Check: &gate}, criterion("ruled", "does_not_ask"),
	}
	m.Rubric = &testlab.Rubric{Items: []testlab.RubricItem{{ID: "plain"}, {ID: "graded"}, {ID: "ruled"}, {ID: "orphan"}}}

	judged := PlanCriteria(m).MaterialForJudge(m)

	if len(judged.Criteria) != 2 || judged.Criteria[0].ID != "plain" || judged.Criteria[1].ID != "graded@0.1" {
		t.Fatalf("sent %+v", judged.Criteria)
	}
	if judged.Instructions["graded@0.1"] != "fail when {item} sounds curt" || judged.Instructions["plain"] != "" {
		t.Errorf("instructions = %v", judged.Instructions)
	}
	var kept []string
	for _, it := range judged.Rubric.Items {
		kept = append(kept, it.ID)
	}
	if !slices.Equal(kept, []string{"plain", "orphan"}) {
		t.Errorf("rubric kept %v, want the sent criterion and the orphan the judge path reports", kept)
	}
}
