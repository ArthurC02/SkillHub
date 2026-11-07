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
					CostUSD: usage.CostUSD, CostReported: usage.CostReported,
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
