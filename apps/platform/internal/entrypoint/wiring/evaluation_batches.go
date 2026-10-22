package wiring

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/batch"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/readmodel"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func WireEvaluation(service *eval.Service, runs *run.Service, registryService *registry.Service) {
	WireEvaluationRunReaders(service, runs)
	WireEvaluationRegistryReaders(service, registryService)
}

func NewEvaluationBatches(
	pool *pgxpool.Pool, runs *run.Service, testlabSvc *testlab.Service, traceSvc *trace.Service,
) *batch.BatchService {
	ports := evaluationBatchPorts(runs, testlabSvc, traceSvc)
	ports.ReadVerdicts = standingVerdictReader(pool)
	return &batch.BatchService{Pool: pool, Ports: ports}
}

func standingVerdictReader(pool *pgxpool.Pool) func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]batch.Verdict, error) {
	return func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]batch.Verdict, error) {
		standing, err := eval.StandingVerdicts(ctx, pool, workspaceID, runIDs)
		out := make(map[pgtype.UUID]batch.Verdict, len(standing))
		for runID, v := range standing {
			out[runID] = batch.Verdict{
				Judged: v.Status == eval.StatusCompleted, Failed: v.Status == eval.StatusFailed, Overall: string(v.Overall),
			}
		}
		return out, err
	}
}

func evaluationBatchPorts(runs *run.Service, testlabSvc *testlab.Service, traceSvc *trace.Service) batch.BatchPorts {
	return batch.BatchPorts{
		StartRun: func(ctx context.Context, b batch.BatchRun) (pgtype.UUID, error) {
			if runs.DispatchGate().Paused(ctx) {
				return pgtype.UUID{}, batch.ErrRunMustWait
			}
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
				return "", false, batch.ErrNotFound
			}
			return hash, confirmed, err
		},
		ReadChainEnds: func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]batch.ChainEnd, error) {
			ends, err := readmodel.Reader{Pool: runs.Pool}.ChainEnds(ctx, workspaceID, runIDs)
			out := make(map[pgtype.UUID]batch.ChainEnd, len(ends))
			for start, end := range ends {
				out[start] = batch.ChainEnd{
					StartSnapshotID: end.StartSnapshotID, RunID: end.RunID, Terminal: end.Terminal,
					Succeeded: end.Succeeded, Failed: end.Failed,
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
		AnswerRun: func(ctx context.Context, a batch.BatchAnswer) error {
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
			return batch.ErrNoFurtherRound
		}
	}
	mapped := batchRunError(err)
	refusal, refused := errors.AsType[batch.RunRefusedError](mapped)
	if refused && refusal.PausesBatch {
		return mapped
	}
	if refused || errors.Is(mapped, batch.ErrTargetGone) {
		return batch.ErrNoFurtherRound
	}
	return mapped
}

func batchRunError(err error) error {
	reason, refused := run.RefusalReason(err)
	switch {
	case err == nil:
		return nil
	case run.RefusalPasses(err):
		return batch.ErrRunMustWait
	case refused:
		return batch.RunRefusedError{Reason: reason, PausesBatch: run.RefusalPausesBatch(err), Err: err}
	case errors.Is(err, run.ErrNotFound), errors.Is(err, run.ErrPreflightTargetNotFound):
		return fmt.Errorf("%w: %w", batch.ErrTargetGone, err)
	default:
		return err
	}
}
