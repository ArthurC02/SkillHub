package batch

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type BatchAnswer struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	RunID       pgtype.UUID
	Answers     []string
}

func SimulatedUserReply(facts []string) string {
	return "補充我剛才沒講的資訊：" + strings.Join(facts, "；") + "。其他照我前面說的。"
}

type unsettledBatch struct {
	id     pgtype.UUID
	status BatchStatus
	actor  pgtype.UUID
	runIDs []pgtype.UUID
}

func (s *BatchService) settleBatches(ctx context.Context, workspaceID pgtype.UUID) error {
	batches, runIDs, err := s.unsettledBatches(ctx, workspaceID)
	if err != nil || len(batches) == 0 {
		return err
	}
	ends, err := s.Ports.ReadChainEnds(ctx, workspaceID, runIDs)
	if err != nil {
		return err
	}
	snapshotIDs := make([]pgtype.UUID, 0, len(ends))
	for _, end := range ends {
		snapshotIDs = append(snapshotIDs, end.StartSnapshotID)
	}
	facts, err := s.Ports.ReadSimulatedUser(ctx, workspaceID, snapshotIDs)
	if err != nil {
		return err
	}
	var failed []error
	for _, batch := range batches {
		err := s.settleBatch(ctx, workspaceID, batch, ends, facts)
		if errors.Is(err, ErrRunMustWait) {
			break
		}
		if err != nil {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

func (s *BatchService) settleBatch(
	ctx context.Context, workspaceID pgtype.UUID, batch *unsettledBatch,
	ends map[pgtype.UUID]ChainEnd, facts map[pgtype.UUID][]string,
) error {
	if batch.status == BatchPaused {
		return nil
	}
	open := batch.status != BatchLaunched
	for _, runID := range batch.runIDs {
		end := ends[runID]
		trialOpen, err := s.answerTrial(ctx, workspaceID, batch.actor, end, facts[end.StartSnapshotID])
		if refusal, refused := errors.AsType[RunRefusedError](err); refused && refusal.PausesBatch {
			return pauseBatch(ctx, s.queries(), workspaceID, batch.id, refusal.Reason)
		}
		if err != nil {
			return err
		}
		open = open || trialOpen
	}
	if open {
		return nil
	}
	_, err := s.queries().MarkBatchSettled(ctx, gen.MarkBatchSettledParams{ID: batch.id, WorkspaceID: workspaceID})
	return err
}

func (s *BatchService) unsettledBatches(ctx context.Context, workspaceID pgtype.UUID) ([]*unsettledBatch, []pgtype.UUID, error) {
	rows, err := s.queries().ListUnsettledBatchTrials(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	var batches []*unsettledBatch
	var runIDs []pgtype.UUID
	for _, row := range rows {
		if len(batches) == 0 || batches[len(batches)-1].id != row.BatchID {
			batches = append(batches, &unsettledBatch{
				id: row.BatchID, status: BatchStatus(row.Status), actor: row.CreatedBy,
			})
		}
		if row.RunID.Valid {
			last := batches[len(batches)-1]
			last.runIDs = append(last.runIDs, row.RunID)
			runIDs = append(runIDs, row.RunID)
		}
	}
	return batches, runIDs, nil
}

func (s *BatchService) answerTrial(
	ctx context.Context, workspaceID, actor pgtype.UUID, end ChainEnd, facts []string,
) (bool, error) {
	if !end.Terminal {
		return true, nil
	}
	if !end.Succeeded || len(facts) == 0 || !actor.Valid {
		return false, nil
	}
	questions, err := s.Ports.ReadQuestionCount(ctx, workspaceID, end.RunID)
	if err != nil || questions == 0 {
		return false, err
	}
	answers := make([]string, questions)
	for i := range answers {
		answers[i] = SimulatedUserReply(facts)
	}
	err = s.Ports.AnswerRun(ctx, BatchAnswer{WorkspaceID: workspaceID, Actor: actor, RunID: end.RunID, Answers: answers})
	if errors.Is(err, ErrNoFurtherRound) {
		return false, nil
	}
	return err == nil, err
}
