package eval

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func repeated(n int, v string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func leaf(kind string, values ...string) testlab.Check {
	return testlab.Check{Kind: kind, Values: values}
}

func node(kind string, children ...testlab.Check) testlab.Check {
	return testlab.Check{Kind: kind, Children: children}
}

func chain(depth int) testlab.Check {
	c := leaf("does_not_ask")
	for range depth - 1 {
		c = node("sequence", c)
	}
	return c
}

func TestACheckIsAcceptedOnlyInTheShapeItsKindTakes(t *testing.T) {
	each := func(atLeast int, items ...string) testlab.Check {
		return testlab.Check{Kind: "each", Values: items, AtLeast: atLeast, Children: []testlab.Check{leaf("contains_values", CheckItem)}}
	}
	cases := []struct {
		name  string
		check testlab.Check
		ok    bool
	}{
		{"contains_values needs at least one value", leaf("contains_values"), false},
		{"contains_values with one value", leaf("contains_values", "v"), true},
		{"contains_values at the value limit", leaf("contains_values", repeated(MaxCheckValues, "v")...), true},
		{"contains_values one past the value limit", leaf("contains_values", repeated(MaxCheckValues+1, "v")...), false},
		{"asks with no values", leaf("asks"), true},
		{"does_not_ask with a value", leaf("does_not_ask", "v"), false},
		{"judge with no values", leaf("judge"), true},
		{"an unknown kind", leaf("regex", "v"), false},
		{"a blank value", leaf("contains_values", "  "), false},
		{"a value at the character limit", leaf("contains_values", strings.Repeat("長", MaxCheckValueChars)), true},
		{"a value one character past the limit", leaf("contains_values", strings.Repeat("長", MaxCheckValueChars+1)), false},
		{"a grader with a child", testlab.Check{Kind: "asks", Children: []testlab.Check{leaf("asks")}}, false},
		{"a sequence with no step", node("sequence"), false},
		{"an if with only its condition", node("if", leaf("asks")), false},
		{"an if with a condition and a branch", node("if", leaf("asks"), leaf("judge")), true},
		{"an if with both branches", node("if", leaf("asks"), leaf("judge"), leaf("does_not_ask")), true},
		{"an if with a fourth child", node("if", leaf("asks"), leaf("judge"), leaf("judge"), leaf("judge")), false},
		{"each needing every item", each(0, "a", "b"), true},
		{"each needing as many as it has", each(2, "a", "b"), true},
		{"each needing one more than it has", each(3, "a", "b"), false},
		{"each needing a negative count", each(-1, "a", "b"), false},
		{"at_least on a node that does not count", testlab.Check{Kind: "all", AtLeast: 1, Children: []testlab.Check{leaf("asks")}}, false},
		{"each with two templates", testlab.Check{Kind: "each", Values: []string{"a"}, Children: []testlab.Check{leaf("asks"), leaf("asks")}}, false},
		{"a tree at the node limit", node("all", repeated49()...), true},
		{"a tree one node past the limit", node("all", append(repeated49(), leaf("asks"))...), false},
		{"a tree at the depth limit", chain(MaxCheckDepth), true},
		{"a tree one level past the depth limit", chain(MaxCheckDepth + 1), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ValidateCheck(&c.check)
			if c.ok && (err != nil || got == nil) {
				t.Fatalf("want accepted, got %v", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidCheck) {
				t.Fatalf("want ErrInvalidCheck, got %v", err)
			}
		})
	}
}

func repeated49() []testlab.Check {
	children := make([]testlab.Check, MaxCheckNodes-1)
	for i := range children {
		children[i] = leaf("asks")
	}
	return children
}

func TestValidationTrimsValuesAllTheWayDown(t *testing.T) {
	if got, err := ValidateCheck(nil); got != nil || err != nil {
		t.Errorf("nil check = %+v, %v", got, err)
	}
	got, err := ValidateCheck(&testlab.Check{Kind: "sequence", Children: []testlab.Check{
		leaf("contains_values", " 1,200 ", "2026-10-11"), leaf("asks"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Children[0].Values, []string{"1,200", "2026-10-11"}) || got.Children[1].Values != nil {
		t.Errorf("children = %+v, want trimmed values and no empty list", got.Children)
	}
}

func TestAtLeastKOfNIsDecidedAsSoonAsTheCountsAllowIt(t *testing.T) {
	cases := []struct {
		name string
		c    counts
		k, n int
		want string
	}{
		{"enough passes", counts{passed: 2, failed: 1}, 2, 3, ResultPassed},
		{"one pass short with nothing left", counts{passed: 1, failed: 2}, 2, 3, ResultFailed},
		{"failures make k unreachable while the judge is pending", counts{failed: 2, pending: 1}, 2, 3, ResultFailed},
		{"still reachable through a pending branch", counts{passed: 1, failed: 1, pending: 1}, 2, 3, resultPending},
		{"still reachable only through an undetermined branch", counts{passed: 1, failed: 1, undetermined: 1}, 2, 3, ResultUndetermined},
		{"pending outranks undetermined", counts{passed: 1, undetermined: 1, pending: 1}, 3, 3, resultPending},
	}
	for _, c := range cases {
		if got := c.c.atLeast(c.k, c.n); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func decide(t *testing.T, m material, check testlab.Check, verdicts map[string]CriterionResult) (CriterionResult, bool) {
	t.Helper()
	return treeResult(m, testlab.Criterion{ID: "c", Text: "c", Check: &check}, verdicts)
}

func TestCompositeChecksCombineTheirChildren(t *testing.T) {
	answered := ruledMaterial("訂金 1,200 元，10月3日出發", nil, "succeeded", true)
	cases := []struct {
		name  string
		check testlab.Check
		want  string
	}{
		{"all with one failing branch", node("all", leaf("contains_values", "1200"), leaf("asks")), ResultFailed},
		{"any with one passing branch", node("any", leaf("contains_values", "999"), leaf("does_not_ask")), ResultPassed},
		{"any with no passing branch", node("any", leaf("contains_values", "999"), leaf("asks")), ResultFailed},
		{"if whose condition held takes its branch", node("if", leaf("does_not_ask"), leaf("contains_values", "999")), ResultFailed},
		{"if whose condition failed takes its else", node("if", leaf("asks"), leaf("judge"), leaf("contains_values", "1200")), ResultPassed},
		{"if whose condition failed with no else requires nothing", node("if", leaf("asks"), leaf("contains_values", "999")), ResultPassed},
		{"each needs two of three items", testlab.Check{Kind: "each", Values: []string{"1200", "2026-10-03", "999"}, AtLeast: 2,
			Children: []testlab.Check{leaf("contains_values", CheckItem)}}, ResultPassed},
		{"each needs every item by default", testlab.Check{Kind: "each", Values: []string{"1200", "999"},
			Children: []testlab.Check{leaf("contains_values", CheckItem)}}, ResultFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, decided := decide(t, answered, c.check, nil)
			if !decided || got.Result != c.want || got.Source != SourceRule {
				t.Fatalf("got %+v (decided %v), want %s from the rules", got, decided, c.want)
			}
		})
	}
}

func TestASequenceStopsAtItsFirstStepThatDidNotPass(t *testing.T) {
	m := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	got, decided := decide(t, m, node("sequence", leaf("contains_values", "999"), leaf("judge"), leaf("asks")), nil)
	if !decided || got.Result != ResultFailed || got.Source != SourceRule {
		t.Fatalf("got %+v, want failed by the rule without waiting for the judge", got)
	}
	if len(got.Steps) != 2 || got.Steps[0].Path != "0.0" || got.Steps[1].Path != "0" {
		t.Errorf("steps = %+v, want only the failing step and the sequence itself", got.Steps)
	}
}

func TestABranchLeftForTheJudgeIsNotRecordedWhenTheRulesAlreadyDecided(t *testing.T) {
	m := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	got, decided := decide(t, m, node("any", leaf("contains_values", "1200"), leaf("judge")), nil)
	if !decided || got.Result != ResultPassed || got.Source != SourceRule {
		t.Fatalf("got %+v (decided %v), want passed by the rule without the judge", got, decided)
	}
	if len(got.Steps) != 2 || got.Steps[0].Path != "0.0" || got.Steps[1].Path != "0" {
		t.Errorf("steps = %+v, want the passing branch and the any, not the unasked judge", got.Steps)
	}
}

func TestAJudgeBehindAPassingGateIsAskedAndDecidesTheCriterion(t *testing.T) {
	m := ruledMaterial("訂金 1,200 元", nil, "succeeded", true)
	gated := node("sequence", leaf("contains_values", "1200"), leaf("judge"))
	if _, decided := decide(t, m, gated, nil); decided {
		t.Fatal("a judge step behind a passing gate was decided without the judge")
	}
	verdict := CriterionResult{Result: ResultFailed, Reason: "the tone is off", Evidence: []EvidenceRef{{Kind: "output"}}}
	got, decided := decide(t, m, gated, map[string]CriterionResult{"c": verdict})
	if !decided || got.Result != ResultFailed || got.Source != SourceModel || len(got.Evidence) != 1 {
		t.Errorf("got %+v, want the judge's failure, labelled model, with its evidence", got)
	}
}
