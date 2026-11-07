package reflection

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
)

func credits(n int64) *int64 { return &n }

func TestARoundIsAdmittedOnlyWhenItsEstimateFitsTheCapAndTheBalance(t *testing.T) {
	quoteErr := errors.New("ledger down")
	for _, tc := range []struct {
		name    string
		halted  bool
		policy  RoundPolicy
		quote   StartQuote
		quoteEr error
		want    error
	}{
		{"one candidate with no cap", false, RoundPolicy{Candidates: 1}, StartQuote{Balance: 70, Threshold: 70}, nil, nil},
		{"one candidate one credit short", false, RoundPolicy{Candidates: 1}, StartQuote{Balance: 69, Threshold: 70}, nil, ErrCreditThreshold},
		{"two candidates with no cap", false, RoundPolicy{Candidates: 2}, StartQuote{Balance: 1000, Threshold: 70}, nil, ErrRoundCapUnset},
		{"two candidates exactly at the cap", false, RoundPolicy{Candidates: 2, RoundCreditCap: credits(140)}, StartQuote{Balance: 140, Threshold: 70}, nil, nil},
		{"two candidates one over the cap", false, RoundPolicy{Candidates: 2, RoundCreditCap: credits(139)}, StartQuote{Balance: 1000, Threshold: 70}, nil, ErrRoundOverCap},
		{"two candidates one credit short of twice the threshold", false, RoundPolicy{Candidates: 2, RoundCreditCap: credits(1000)}, StartQuote{Balance: 139, Threshold: 70}, nil, ErrCreditThreshold},
		{"one candidate over a set cap", false, RoundPolicy{Candidates: 1, RoundCreditCap: credits(69)}, StartQuote{Balance: 1000, Threshold: 70}, nil, ErrRoundOverCap},
		{"halted before anything", true, RoundPolicy{Candidates: 2}, StartQuote{}, nil, ErrModelCallsHalted},
		{"the quote fails", false, RoundPolicy{Candidates: 1}, StartQuote{}, quoteErr, quoteErr},
	} {
		quoted := false
		b := Billing{
			ModelCallsHalted: func(context.Context) bool { return tc.halted },
			Quote: func(context.Context, pgtype.UUID) (StartQuote, error) {
				quoted = true
				return tc.quote, tc.quoteEr
			},
		}
		err := b.admit(context.Background(), uuidN(1), tc.policy)
		if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if (errors.Is(tc.want, ErrRoundCapUnset) || errors.Is(tc.want, ErrModelCallsHalted)) && quoted {
			t.Errorf("%s: the balance was read for a round refused before it", tc.name)
		}
	}
}

func TestEachCandidateOfARoundIsGivenItsOwnFocusInOrder(t *testing.T) {
	for _, tc := range []struct {
		candidates int
		want       []string
	}{
		{1, []string{""}},
		{2, []string{"instructions", "scripts_and_tools"}},
		{3, []string{"instructions", "scripts_and_tools", "references_and_examples"}},
		{4, []string{"instructions", "scripts_and_tools", "references_and_examples", "free"}},
	} {
		if got := roundFocuses(tc.candidates); !slices.Equal(got, tc.want) {
			t.Errorf("%d candidates: focuses = %q, want %q", tc.candidates, got, tc.want)
		}
	}
}

type chargeLog struct{ usages []*modelport.ModelUsage }

func (c *chargeLog) service() *Service {
	return &Service{Billing: Billing{Charge: func(_ context.Context, _ pgtype.UUID, u *modelport.ModelUsage) error {
		c.usages = append(c.usages, u)
		return nil
	}}}
}

func answered(prompt int64, usd float64, proposals int) candidateCall {
	return candidateCall{resp: &modelport.Improvements{
		Proposals: make([]modelport.ImprovementProposal, proposals),
		Usage:     &modelport.ModelUsage{PromptTokens: prompt, CompletionTokens: 1, CostUSD: &usd, CostReported: true},
	}}
}

func TestARoundIsChargedOnceForTheSumOfItsAnsweredCalls(t *testing.T) {
	var log chargeLog
	round := &eval.NewReflection{}
	focuses := []string{"instructions", "scripts_and_tools", "references_and_examples"}
	calls := []candidateCall{answered(20, 0.01, 2), {err: errors.New("gateway 502")}, answered(30, 0.02, 0)}

	if err := log.service().settleRound(context.Background(), round, focuses, calls); err != nil {
		t.Fatal(err)
	}
	if len(log.usages) != 1 {
		t.Fatalf("charged %d times, want once", len(log.usages))
	}
	u := log.usages[0]
	if u.PromptTokens != 50 || u.CompletionTokens != 2 || !u.CostReported || *u.CostUSD != 0.03 {
		t.Errorf("charged usage = %+v (cost %v), want 50 prompt, 2 completion, 0.03 reported", u, *u.CostUSD)
	}
	if !slices.Equal(u.CallCostsUSD, []float64{0.01, 0.02}) {
		t.Errorf("call costs = %v, want each answered call's 0.01 and 0.02", u.CallCostsUSD)
	}
	want := []eval.ReflectionCandidate{
		{Focus: "instructions", Answered: true}, {Focus: "scripts_and_tools"}, {Focus: "references_and_examples", Answered: true},
	}
	for i, c := range round.Candidates {
		if c.Focus != want[i].Focus || c.Answered != want[i].Answered {
			t.Errorf("candidate %d = %+v, want %+v", i, c, want[i])
		}
	}
	if len(round.Candidates[0].Proposals) != 2 || len(round.Candidates[2].Proposals) != 0 {
		t.Errorf("proposals kept per candidate = %d and %d, want 2 and 0", len(round.Candidates[0].Proposals), len(round.Candidates[2].Proposals))
	}
}

func TestARoundWithNoAnsweredCallIsNotCharged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		calls []candidateCall
		want  error
	}{
		{"every call failed", []candidateCall{{err: errors.New("502")}, {err: errors.New("timeout")}}, nil},
		{"the platform budget ran out", []candidateCall{{err: modelport.ErrModelBudgetExhausted}, {err: errors.New("502")}}, ErrBudgetExhausted},
	} {
		var log chargeLog
		round := &eval.NewReflection{}
		err := log.service().settleRound(context.Background(), round, []string{"instructions", "free"}, tc.calls)
		if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if len(log.usages) != 0 {
			t.Errorf("%s: charged %d times, want never", tc.name, len(log.usages))
		}
	}
}

func TestARoundWhoseBudgetRanOutMidwayStillKeepsAndChargesWhatWasAnswered(t *testing.T) {
	var log chargeLog
	round := &eval.NewReflection{}
	calls := []candidateCall{{err: modelport.ErrModelBudgetExhausted}, answered(20, 0.01, 1)}
	if err := log.service().settleRound(context.Background(), round, []string{"instructions", "scripts_and_tools"}, calls); err != nil {
		t.Fatalf("err = %v, want the answered candidate kept", err)
	}
	if len(log.usages) != 1 || round.Candidates[0].Answered || !round.Candidates[1].Answered {
		t.Errorf("charges = %d, candidates = %+v; want one charge and only the second answered", len(log.usages), round.Candidates)
	}
}

func TestARoundsCostIsReportedOnlyWhenEveryAnsweredCallReportedIt(t *testing.T) {
	usd := 0.01
	reported := &modelport.ModelUsage{PromptTokens: 5, CostUSD: &usd, CostReported: true}
	unreported := &modelport.ModelUsage{PromptTokens: 7}
	if got := roundUsage([]*modelport.ModelUsage{reported, unreported}); got.PromptTokens != 12 || got.CostReported || got.CostUSD != nil || got.CallCostsUSD != nil {
		t.Errorf("mixed = %+v, want 12 tokens and no reported cost", got)
	}
	if got := roundUsage([]*modelport.ModelUsage{reported, nil}); got.PromptTokens != 5 || got.CostReported {
		t.Errorf("one call without usage = %+v, want 5 tokens and no reported cost", got)
	}
	if got := roundUsage([]*modelport.ModelUsage{nil, nil}); got != nil {
		t.Errorf("no usage at all = %+v, want nil", got)
	}
}

func TestASingleReportedCallListsNoCallCosts(t *testing.T) {
	usd := 0.01
	if got := roundUsage([]*modelport.ModelUsage{{PromptTokens: 5, CostUSD: &usd, CostReported: true}}); !got.CostReported || got.CallCostsUSD != nil {
		t.Errorf("one call = %+v, want its reported cost and no call list", got)
	}
}
