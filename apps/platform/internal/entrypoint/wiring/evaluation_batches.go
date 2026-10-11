package wiring

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func WireEvaluation(service *eval.Service, runs *run.Service, registryService *registry.Service) {
	WireEvaluationRunReaders(service, runs)
	WireEvaluationRegistryReaders(service, registryService)
}

func NewEvaluationBatches(
	pool *pgxpool.Pool, runs *run.Service, testlabSvc *testlab.Service, traceSvc *trace.Service,
) *eval.BatchService {
	return &eval.BatchService{Pool: pool, Ports: evaluationBatchPorts(runs, testlabSvc, traceSvc)}
}

func evaluationBatchPorts(runs *run.Service, testlabSvc *testlab.Service, traceSvc *trace.Service) eval.BatchPorts {
	return eval.BatchPorts{
		StartRun: func(ctx context.Context, b eval.BatchRun) (pgtype.UUID, error) {
			created, err := runs.Create(ctx, run.CreateParams{
				WorkspaceID: b.WorkspaceID, Actor: b.Actor, SkillID: b.SkillID,
				VersionID: b.VersionID, TestCaseID: b.TestCaseID, ConfirmedSummaryHash: b.SummaryHash,
				WithholdSkill: b.WithholdSkill, OriginKey: b.OriginKey,
			})
			return created.ID, batchRunError(err)
		},
		ConfirmedSummaryHash: func(ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID) (string, bool, error) {
			hash, confirmed, err := runs.ConfirmedPermissionHash(ctx, workspaceID, skillID, versionID, testCaseID)
			if errors.Is(err, run.ErrNotFound) || errors.Is(err, run.ErrPreflightTargetNotFound) {
				return "", false, eval.ErrNotFound
			}
			return hash, confirmed, err
		},
		ReadChainEnds: func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]eval.ChainEnd, error) {
			ends, err := runs.ChainEnds(ctx, workspaceID, runIDs)
			out := make(map[pgtype.UUID]eval.ChainEnd, len(ends))
			for start, end := range ends {
				out[start] = eval.ChainEnd{
					StartSnapshotID: end.StartSnapshotID, RunID: end.RunID, Status: end.Status, Terminal: end.Terminal,
				}
			}
			return out, err
		},
		ReadSnapshotContents: testlabSvc.SnapshotContentHashes,
		ReadQuestionCount: func(ctx context.Context, workspaceID, runID pgtype.UUID) (int, error) {
			summary, err := traceSvc.General(ctx, workspaceID, runID)
			return len(summary.Questions), err
		},
		ReadSimulatedUser: testlabSvc.SimulatedUserFacts,
		AnswerRun: func(ctx context.Context, a eval.BatchAnswer) error {
			_, err := runs.Continue(ctx, run.ContinueParams{
				WorkspaceID: a.WorkspaceID, Actor: a.Actor, RunID: a.RunID, Answers: a.Answers,
				AnsweredAsSimulatedUser: true,
			})
			return batchAnswerError(err)
		},
	}
}

var noFurtherRound = []error{
	run.ErrNothingToAnswer, run.ErrAnswersDoNotFit, run.ErrContinuationLimit, run.ErrContinuationTooLarge,
}

func batchAnswerError(err error) error {
	if errors.Is(err, run.ErrAlreadyContinued) {
		return nil
	}
	for _, final := range noFurtherRound {
		if errors.Is(err, final) {
			return eval.ErrNoFurtherRound
		}
	}
	mapped := batchRunError(err)
	if _, refused := errors.AsType[eval.RunRefusedError](mapped); refused {
		return eval.ErrNoFurtherRound
	}
	return mapped
}

func batchRunError(err error) error {
	reason, refused := run.RefusalReason(err)
	switch {
	case err == nil:
		return nil
	case run.RefusalPasses(err):
		return eval.ErrRunMustWait
	case refused:
		return eval.RunRefusedError{Reason: reason, Err: err}
	case errors.Is(err, run.ErrNotFound), errors.Is(err, run.ErrPreflightTargetNotFound):
		return eval.RunRefusedError{Reason: "target_gone", Err: err}
	default:
		return err
	}
}
