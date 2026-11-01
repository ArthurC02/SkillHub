package judging

import (
	"context"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
)

const InjectionLine = 0.5

const tokensPerMillion = 1_000_000

const screeningTimeout = 15 * time.Second // budget-over: injection.TIMEOUT_SECONDS

type ScreeningStatus string

const (
	ScreeningFlagged   ScreeningStatus = "flagged"
	ScreeningClear     ScreeningStatus = "clear"
	ScreeningUnchecked ScreeningStatus = "unchecked"
)

type ScreeningPolicy struct {
	On                  bool
	USDPerMillionTokens float64
}

type Screening struct {
	Status      ScreeningStatus
	Probability *float64
	Model       string
	InputTokens int64
	CostUSD     *float64
}

func (j Judges) screen(ctx context.Context, req modelport.JudgeRequest) *Screening {
	if j.Screener == nil || j.ScreeningPolicy == nil {
		return nil
	}
	policy, err := j.ScreeningPolicy(ctx)
	if err != nil {
		slog.Warn("judging: the injection check setting could not be read; this evaluation records the check as not run",
			"evaluation_id", req.EvaluationID, "error", err)
		return &Screening{Status: ScreeningUnchecked}
	}
	if !policy.On {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, screeningTimeout)
	defer cancel()
	answer, err := j.Screener.Screen(callCtx, modelport.ScreeningRequest{
		RunID: req.RunID, EvaluationID: req.EvaluationID,
		FinalOutput: req.FinalOutput, Artifacts: req.Artifacts, TraceDigest: req.TraceDigest,
	})
	if err != nil || answer == nil || !(answer.Probability >= 0 && answer.Probability <= 1) || answer.InputTokens < 0 {
		slog.Warn("judging: the injection check did not answer; the verdict is decided without it",
			"evaluation_id", req.EvaluationID, "error", err)
		return &Screening{Status: ScreeningUnchecked}
	}
	return screeningOf(*answer, policy)
}

func screeningOf(answer modelport.ScreeningAnswer, policy ScreeningPolicy) *Screening {
	status := ScreeningClear
	if answer.Probability >= InjectionLine {
		status = ScreeningFlagged
	}
	cost := float64(answer.InputTokens) * policy.USDPerMillionTokens / tokensPerMillion
	return &Screening{
		Status: status, Probability: &answer.Probability,
		Model: answer.Model, InputTokens: answer.InputTokens, CostUSD: &cost,
	}
}

func (s *Screening) Clone() *Screening {
	if s == nil {
		return nil
	}
	c := *s
	if s.Probability != nil {
		p := *s.Probability
		c.Probability = &p
	}
	if s.CostUSD != nil {
		usd := *s.CostUSD
		c.CostUSD = &usd
	}
	return &c
}

type Options struct {
	PanelEnabled    func(context.Context) (bool, error)
	Screener        modelport.Screener
	ScreeningPolicy func(context.Context) (ScreeningPolicy, error)
}
