package judging

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

type fakeScreener struct {
	answer *modelport.ScreeningAnswer
	err    error
	asked  []modelport.ScreeningRequest
}

func (f *fakeScreener) Screen(_ context.Context, req modelport.ScreeningRequest) (*modelport.ScreeningAnswer, error) {
	f.asked = append(f.asked, req)
	return f.answer, f.err
}

func pricedAt(usdPerMillion float64) func(context.Context) (ScreeningPolicy, error) {
	return func(context.Context) (ScreeningPolicy, error) {
		return ScreeningPolicy{On: true, USDPerMillionTokens: usdPerMillion}, nil
	}
}

func singleJudge() *panelOfFakes {
	return &panelOfFakes{byRole: map[string]*modelport.Judgement{
		"": said("judge", verdict.ResultPassed, realQuote, verdict.ResultPassed, realFile),
	}}
}

func screenedOutcome(t *testing.T, j Judges) Outcome {
	t.Helper()
	m, _ := fixtureMaterial(true)
	v, err := j.Assess(context.Background(), m, "eval-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTheScreeningAsksAboutTheSameUntrustedMaterialTheJudgeReads(t *testing.T) {
	judge := singleJudge()
	screener := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.1, Model: "m", InputTokens: 10}}
	screenedOutcome(t, Judges{Judge: judge, Screener: screener, ScreeningPolicy: pricedAt(1)})

	m, _ := fixtureMaterial(true)
	judged, _ := buildRequest(m, "eval-1")
	if len(screener.asked) != 1 {
		t.Fatalf("screened %d times, want once per evaluation", len(screener.asked))
	}
	asked := screener.asked[0]
	if asked.EvaluationID != "eval-1" || asked.RunID != judged.RunID || asked.FinalOutput != judged.FinalOutput ||
		len(asked.Artifacts) != len(judged.Artifacts) || len(asked.TraceDigest.Entries) != len(judged.TraceDigest.Entries) {
		t.Errorf("screened %+v, want the judge's run, output, artifacts and trace digest", asked)
	}
}

func TestTheScreeningIsOneCallEvenWhenAPanelJudges(t *testing.T) {
	fakes := &panelOfFakes{byRole: map[string]*modelport.Judgement{}}
	for _, role := range PanelRoles {
		fakes.byRole[role] = said(role, verdict.ResultPassed, realQuote, verdict.ResultPassed, realFile)
	}
	screener := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.1, Model: "m", InputTokens: 10}}
	screenedOutcome(t, Judges{Judge: fakes, PanelEnabled: panelOn, Screener: screener, ScreeningPolicy: pricedAt(1)})

	if len(screener.asked) != 1 {
		t.Errorf("screened %d times under a panel, want once", len(screener.asked))
	}
}

func TestAnAnswerAtTheLineIsFlaggedAndOneJustBelowIsClear(t *testing.T) {
	for _, tc := range []struct {
		name        string
		probability float64
		want        ScreeningStatus
	}{
		{"zero", 0, ScreeningClear},
		{"just below the line", math.Nextafter(InjectionLine, 0), ScreeningClear},
		{"on the line", 0.5, ScreeningFlagged},
		{"certain", 1, ScreeningFlagged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screener := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: tc.probability, Model: "luna", InputTokens: 1820}}
			got := screenedOutcome(t, Judges{Judge: singleJudge(), Screener: screener, ScreeningPolicy: pricedAt(0.25)}).Screening
			if got == nil || got.Status != tc.want || got.Probability == nil || *got.Probability != tc.probability {
				t.Fatalf("screening = %+v, want %s at %v", got, tc.want, tc.probability)
			}
			if got.Model != "luna" || got.InputTokens != 1820 || got.CostUSD == nil || *got.CostUSD != 0.000455 {
				t.Errorf("screening = %+v, want luna, 1820 tokens costing 0.000455 at 0.25 per million", got)
			}
		})
	}
}

func TestTheScreeningRecordsNothingWhenItIsSwitchedOffOrNotWired(t *testing.T) {
	answered := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.9, InputTokens: 5}}
	off := func(context.Context) (ScreeningPolicy, error) {
		return ScreeningPolicy{On: false, USDPerMillionTokens: 1}, nil
	}
	for _, tc := range []struct {
		name string
		j    Judges
	}{
		{"switched off", Judges{Screener: answered, ScreeningPolicy: off}},
		{"no screener in this deployment", Judges{ScreeningPolicy: pricedAt(1)}},
		{"no setting wired", Judges{Screener: answered}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(answered.asked)
			tc.j.Judge = singleJudge()
			if got := screenedOutcome(t, tc.j).Screening; got != nil {
				t.Errorf("screening = %+v, want none", got)
			}
			if len(answered.asked) != before {
				t.Error("the screener was asked although the check is not on")
			}
		})
	}
}

func TestAScreeningThatDoesNotAnswerIsRecordedAsUncheckedNotClear(t *testing.T) {
	unreadable := func(context.Context) (ScreeningPolicy, error) {
		return ScreeningPolicy{}, errors.New("settings unreadable")
	}
	for _, tc := range []struct {
		name     string
		screener *fakeScreener
		policy   func(context.Context) (ScreeningPolicy, error)
	}{
		{"the setting cannot be read", &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.1}}, unreadable},
		{"the call fails", &fakeScreener{err: errors.New("503")}, pricedAt(1)},
		{"no answer", &fakeScreener{}, pricedAt(1)},
		{"probability above one", &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 1.01}}, pricedAt(1)},
		{"probability below zero", &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: -0.01}}, pricedAt(1)},
		{"probability not a number", &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: math.NaN()}}, pricedAt(1)},
		{"negative tokens", &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.1, InputTokens: -1}}, pricedAt(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := screenedOutcome(t, Judges{Judge: singleJudge(), Screener: tc.screener, ScreeningPolicy: tc.policy}).Screening
			if got == nil || got.Status != ScreeningUnchecked || got.Probability != nil || got.CostUSD != nil {
				t.Errorf("screening = %+v, want unchecked with no probability and no cost", got)
			}
		})
	}
}

func TestAFlaggedScreeningLeavesTheVerdictAsTheJudgeDecidedIt(t *testing.T) {
	screener := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 1, InputTokens: 5}}
	plain := screenedOutcome(t, Judges{Judge: singleJudge()})
	flagged := screenedOutcome(t, Judges{Judge: singleJudge(), Screener: screener, ScreeningPolicy: pricedAt(1)})

	if flagged.Screening == nil || flagged.Screening.Status != ScreeningFlagged {
		t.Fatalf("screening = %+v, want flagged", flagged.Screening)
	}
	for i := range plain.Results {
		if flagged.Results[i].Result != plain.Results[i].Result {
			t.Errorf("criterion %s = %s when flagged, want %s as without the check",
				plain.Results[i].CriterionID, flagged.Results[i].Result, plain.Results[i].Result)
		}
	}
}

func TestAFailedJudgementReturnsNoOutcomeWhateverTheScreeningSaid(t *testing.T) {
	failing := &panelOfFakes{failing: "", byRole: map[string]*modelport.Judgement{}}
	failing.failing = "skillhub-judge"
	m, _ := fixtureMaterial(true)
	screener := &fakeScreener{answer: &modelport.ScreeningAnswer{Probability: 0.1, InputTokens: 5}}
	_, err := Judges{Judge: failing, PanelEnabled: panelOn, Screener: screener, ScreeningPolicy: pricedAt(1)}.Assess(context.Background(), m, "", 0)
	if err == nil {
		t.Error("a judge that did not answer produced an outcome")
	}
}
