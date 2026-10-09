package eval

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type panelOfFakes struct {
	mu      sync.Mutex
	byRole  map[string]*Judgement
	failing string
	asked   []string
}

func (p *panelOfFakes) JudgeRun(_ context.Context, req JudgeRequest) (*Judgement, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, req.ModelRole)
	if p.failing != "" && req.ModelRole == p.failing {
		return nil, errors.New("gateway error")
	}
	return p.byRole[req.ModelRole], nil
}

var realQuote = []Citation{{Kind: KindAgentOutput, Quote: "Removed 17 duplicate rows"}}

var realFile = []Citation{{Kind: KindArtifact, ArtifactPath: strp("output.xlsx")}}

func said(model, c1 string, c1Cites []Citation, c2 string, c2Cites []Citation) *Judgement {
	return &Judgement{
		Model: model, PromptVersion: "judge-run/v3", Summary: model + " summary",
		Criteria: []CriterionVerdict{
			{CriterionID: "c1", Result: c1, Reason: model + " on c1", Citations: c1Cites},
			{CriterionID: "c2", Result: c2, Reason: model + " on c2", Citations: c2Cites},
		},
	}
}

func judgedByPanel(t *testing.T, members ...*Judgement) (verdict, *panelOfFakes) {
	t.Helper()
	fakes := &panelOfFakes{byRole: map[string]*Judgement{}}
	for i, member := range members {
		fakes.byRole[PanelRoles[i]] = member
	}
	s := &Service{Judge: fakes, JudgePanel: PanelRoles}
	m, _ := fixtureMaterial(true)
	v, err := s.judge(context.Background(), m, gen.Evaluation{})
	if err != nil {
		t.Fatal(err)
	}
	return v, fakes
}

func TestAPanelRecordsTheMajorityResultWithTheEvidenceOfAMemberWhoGaveIt(t *testing.T) {
	v, fakes := judgedByPanel(t,
		said("a", ResultFailed, realQuote, ResultFailed, realFile),
		said("b", ResultPassed, realQuote, ResultFailed, realFile),
		said("c", ResultPassed, realQuote, ResultPassed, realFile),
	)
	if got := slices.Sorted(slices.Values(fakes.asked)); !slices.Equal(got, PanelRoles) {
		t.Fatalf("asked roles %v, want each panel role once", got)
	}
	if v.results[0].Result != ResultPassed || v.results[0].Reason != "b on c1" || len(v.results[0].Evidence) == 0 {
		t.Errorf("c1 = %+v, want b's verified pass", v.results[0])
	}
	if v.results[1].Result != ResultFailed || v.results[1].Reason != "a on c2" {
		t.Errorf("c2 = %+v, want a's failure", v.results[1])
	}
	if len(v.findings) != 0 {
		t.Errorf("a panel with a majority on every criterion reports no split, got %+v", v.findings)
	}
	if v.model != "a + b + c" {
		t.Errorf("model = %q, want every member named", v.model)
	}
}

func TestACriterionThePanelSplitsOnIsUndeterminedAndNamed(t *testing.T) {
	v, _ := judgedByPanel(t,
		said("a", ResultPassed, realQuote, ResultFailed, realFile),
		said("b", ResultFailed, realQuote, ResultFailed, realFile),
		said("c", ResultUndetermined, nil, ResultFailed, realFile),
	)
	c1 := v.results[0]
	if c1.Result != ResultUndetermined || len(c1.Evidence) != 0 {
		t.Fatalf("c1 = %+v, want undetermined with no one member's evidence", c1)
	}
	if !strings.Contains(c1.Reason, "passed by 1, failed by 1, undetermined by 1") {
		t.Errorf("c1 reason %q does not give the tally", c1.Reason)
	}
	if v.results[1].Result != ResultFailed {
		t.Errorf("c2 = %+v, want the unanimous failure", v.results[1])
	}
	if len(v.findings) != 1 || !strings.Contains(v.findings[0].Message, "c1") || strings.Contains(v.findings[0].Message, "c2") {
		t.Errorf("findings %+v, want one naming only c1", v.findings)
	}
	if v.overall != overallFrom(v.results) {
		t.Errorf("overall %q is not recomputed from the voted results", v.overall)
	}
}

func TestThePanelVotesOnVerifiedResultsNotOnWhatTheMembersClaimed(t *testing.T) {
	invented := []Citation{{Kind: KindAgentOutput, Quote: "every row was checked by hand"}}
	v, _ := judgedByPanel(t,
		said("a", ResultPassed, realQuote, ResultFailed, realFile),
		said("b", ResultPassed, invented, ResultFailed, realFile),
		said("c", ResultPassed, invented, ResultFailed, realFile),
	)
	if v.results[0].Result != ResultUndetermined || strings.Contains(v.results[0].Reason, "did not agree") {
		t.Errorf("c1 = %+v, want the two unverifiable passes to outvote the verified one", v.results[0])
	}
}

func TestAPanelMemberThatDoesNotAnswerLeavesThePanelWithoutAVerdict(t *testing.T) {
	fakes := &panelOfFakes{failing: PanelRoles[1], byRole: map[string]*Judgement{
		PanelRoles[0]: said("a", ResultPassed, realQuote, ResultFailed, realFile),
		PanelRoles[2]: said("c", ResultPassed, realQuote, ResultFailed, realFile),
	}}
	m, _ := fixtureMaterial(true)
	if _, err := (&Service{Judge: fakes, JudgePanel: PanelRoles}).judge(context.Background(), m, gen.Evaluation{}); err == nil {
		t.Fatal("a panel missing a member returned a verdict")
	}
}

func TestWithoutAPanelTheJudgeIsAskedOnceInItsOwnRole(t *testing.T) {
	fakes := &panelOfFakes{byRole: map[string]*Judgement{
		"": said("solo", ResultPassed, realQuote, ResultFailed, realFile),
	}}
	m, _ := fixtureMaterial(true)
	v, err := (&Service{Judge: fakes}).judge(context.Background(), m, gen.Evaluation{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fakes.asked, []string{""}) || v.model != "solo" || v.summary != "solo summary" {
		t.Fatalf("asked %q, model %q, summary %q; want one call in the service's own role", fakes.asked, v.model, v.summary)
	}
}

func TestAnEvenSplitIsNotAMajority(t *testing.T) {
	passed := CriterionResult{CriterionID: "c1", Result: ResultPassed}
	failed := CriterionResult{CriterionID: "c1", Result: ResultFailed}
	results, split := vote([][]CriterionResult{{passed}, {failed}})
	if results[0].Result != ResultUndetermined || !slices.Equal(split, []string{"c1"}) {
		t.Fatalf("one pass against one failure gave %+v, split %v; want undetermined", results[0], split)
	}
}

func TestPanelUsageAddsTheMembersAndAnUnpricedMemberLeavesTheCostUnknown(t *testing.T) {
	priced := func(tokens int64, usd float64) *Judgement {
		return &Judgement{Usage: &ModelUsage{PromptTokens: tokens, CompletionTokens: 1, CostUSD: &usd, CostReported: true}}
	}
	sum := panelUsage([]*Judgement{priced(10, 0.25), priced(20, 0.5)})
	if sum.PromptTokens != 30 || sum.CompletionTokens != 2 || sum.ReportedCostUSD() == nil || *sum.ReportedCostUSD() != 0.75 {
		t.Errorf("sum = %+v, want 30/2 tokens and 0.75 USD", sum)
	}
	unpriced := &Judgement{Usage: &ModelUsage{PromptTokens: 5}}
	if got := panelUsage([]*Judgement{priced(10, 0.25), unpriced}); got.ReportedCostUSD() != nil || got.PromptTokens != 15 {
		t.Errorf("with an unpriced member got %+v, want tokens counted and the cost unknown", got)
	}
	if got := panelUsage([]*Judgement{priced(10, 0.25), {}}); got.ReportedCostUSD() != nil {
		t.Errorf("a member with no usage at all still left a cost: %+v", got)
	}
}
