package apiserver

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/batch"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/evalsettings"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/reflection"
)

func newReflectionHandler(cfg Config, app *App, creditSvc *credit.Service) *reflection.Handler {
	billing := suggestBilling(testlab.SuggestBilling{}, credit.KindBatchReflection, creditSvc, app.Auth.Service, cfg.Pool)
	settings := &evalsettings.Service{Pool: cfg.Pool}
	svc := &reflection.Service{
		Eval:            app.EvalSvc,
		ReadBatchTrials: batchTrialsReader(app.EvalBatches),
		Candidates:      candidatePorts(app, creditSvc),
		Policy: func(ctx context.Context) (reflection.RoundPolicy, error) {
			policy, err := settings.ReflectionPolicy(ctx)
			return reflection.RoundPolicy{Candidates: policy.Count, RoundCreditCap: policy.RoundCreditCap}, err
		},
		Billing: reflection.Billing{
			ModelCallsHalted: app.RunSvc.DispatchGate().ModelCallsHalted,
			Quote: func(ctx context.Context, workspaceID pgtype.UUID) (reflection.StartQuote, error) {
				userID, err := app.Auth.Service.WorkspaceOwner(ctx, workspaceID)
				if err != nil {
					return reflection.StartQuote{}, err
				}
				check, err := creditSvc.CanStart(ctx, userID, credit.KindBatchReflection)
				return reflection.StartQuote{Balance: check.Balance, Threshold: check.Threshold}, err
			},
			Charge: func(ctx context.Context, workspaceID pgtype.UUID, usage *modelport.ModelUsage) error {
				if usage == nil {
					return billing.Charge(ctx, workspaceID, nil)
				}
				return billing.Charge(ctx, workspaceID, &testlab.ModelUsage{
					PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
					CostUSD: usage.CostUSD, CostReported: usage.CostReported, CallCostsUSD: usage.CallCostsUSD,
				})
			},
		},
	}
	return &reflection.Handler{Svc: svc, Identity: app.Auth.Service}
}

type batchTrialsRead func(ctx context.Context, workspaceID, batchID pgtype.UUID) (reflection.BatchTrials, bool, error)

func batchTrialsReader(batches *batch.BatchService) batchTrialsRead {
	return func(ctx context.Context, workspaceID, batchID pgtype.UUID) (reflection.BatchTrials, bool, error) {
		report, err := batches.ReportOfBatch(ctx, workspaceID, batchID)
		if errors.Is(err, batch.ErrNotFound) {
			return reflection.BatchTrials{}, false, nil
		}
		if err != nil {
			return reflection.BatchTrials{}, false, err
		}
		out := reflection.BatchTrials{
			SkillID: report.Batch.SkillID, VersionID: report.Batch.VersionID,
			Launching: report.Batch.Status == batch.BatchLaunching,
			Trials:    report.Batch.Trials, WithSkillWithheld: report.Batch.WithSkillWithheld,
		}
		for _, tc := range report.TestCases {
			cases := reflection.BatchTestCaseTrials{TestCaseID: tc.TestCaseID}
			for _, trial := range tc.Trials {
				cases.Trials = append(cases.Trials, reflection.BatchTrial{EndRunID: trial.EndRunID, Settled: trial.Outcome.Settled()})
			}
			out.TestCases = append(out.TestCases, cases)
		}
		return out, true, nil
	}
}

func candidatePorts(app *App, creditSvc *credit.Service) reflection.CandidatePorts {
	batches := app.EvalBatches
	return reflection.CandidatePorts{
		Versions: func(ctx context.Context, workspaceID, skillID pgtype.UUID) ([]pgtype.UUID, error) {
			versions, err := app.Deps.Registry.Svc.Versions(ctx, workspaceID, skillID)
			ids := make([]pgtype.UUID, len(versions))
			for i, v := range versions {
				ids[i] = v.ID
			}
			return ids, err
		},
		Confirmed: func(ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID) (bool, error) {
			_, confirmed, err := batches.Ports.ConfirmedSummaryHash(ctx, workspaceID, skillID, versionID, testCaseID)
			return confirmed, err
		},
		Launch: func(ctx context.Context, l reflection.CandidateLaunch) (pgtype.UUID, error) {
			key := l.OriginKey
			created, err := batches.CreateBatch(ctx, batch.NewBatch{
				WorkspaceID: l.WorkspaceID, Actor: l.Actor, SkillID: l.SkillID, VersionID: l.VersionID,
				TestCaseIDs: l.TestCaseIDs, Trials: l.Trials, WithSkillWithheld: l.WithSkillWithheld, OriginKey: &key,
			})
			return created.ID, err
		},
		RunQuote: func(ctx context.Context, workspaceID pgtype.UUID) (reflection.StartQuote, error) {
			userID, err := app.Auth.Service.WorkspaceOwner(ctx, workspaceID)
			if err != nil {
				return reflection.StartQuote{}, err
			}
			check, err := creditSvc.CanStart(ctx, userID, credit.KindRun)
			return reflection.StartQuote{Balance: check.Balance, Threshold: check.Threshold}, err
		},
		Compare: func(ctx context.Context, workspaceID, candidateBatch, sourceBatch pgtype.UUID, dev []pgtype.UUID) (reflection.CandidateComparison, error) {
			return candidateComparer{batches}.compare(ctx, workspaceID, candidateBatch, sourceBatch, dev)
		},
	}
}

type candidateComparer struct{ batches *batch.BatchService }

func (c candidateComparer) compare(
	ctx context.Context, workspaceID, candidateBatch, sourceBatch pgtype.UUID, dev []pgtype.UUID,
) (reflection.CandidateComparison, error) {
	batches := c.batches
	candidate, err := batches.ReportOfBatch(ctx, workspaceID, candidateBatch)
	if err != nil {
		return reflection.CandidateComparison{}, err
	}
	source, err := batches.ReportOfBatch(ctx, workspaceID, sourceBatch)
	if err != nil {
		return reflection.CandidateComparison{}, err
	}
	devCases, heldOut := map[pgtype.UUID]bool{}, map[pgtype.UUID]bool{}
	for _, id := range dev {
		devCases[id] = true
	}
	for _, tc := range source.TestCases {
		if !devCases[tc.TestCaseID] {
			heldOut[tc.TestCaseID] = true
		}
	}
	return reflection.CandidateComparison{
		Settled: everyTrialSettled(candidate),
		Rates:   batch.JudgedRates(candidate.ArmReport), Source: batch.JudgedRates(source.ArmReport),
		Dev:     pairedVerdict(batch.PairArms(candidate.On(devCases), source.On(devCases))),
		HeldOut: pairedVerdict(batch.PairArms(candidate.On(heldOut), source.On(heldOut))),
	}, nil
}

func everyTrialSettled(report batch.BatchReport) bool {
	if report.Batch.Status == batch.BatchStopped {
		return true
	}
	for _, tc := range report.TestCases {
		for _, trial := range tc.Trials {
			if !trial.Outcome.Settled() {
				return false
			}
		}
	}
	return report.Batch.Status != batch.BatchLaunching
}

func pairedVerdict(p batch.PairedDifference) reflection.PairedVerdict {
	out := reflection.PairedVerdict{
		Mean: p.Difference, Verdict: p.Verdict,
		AtLeastLikelyImproved: p.Verdict == batch.VerdictImproved || p.Verdict == batch.VerdictLikelyImproved,
	}
	if p.Difference95 != nil {
		out.Low, out.High = &p.Difference95.Low, &p.Difference95.High
	}
	return out
}
