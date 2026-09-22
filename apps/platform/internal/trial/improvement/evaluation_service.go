package eval

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (s *Service) Current(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationRecord, error) {
	ev, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationRecord{}, ErrNotFound
	}
	return evaluationRecordOf(ev), err
}

func (s *Service) Revision(ctx context.Context, workspaceID, runID, id pgtype.UUID) (EvaluationRecord, error) {
	ev, err := s.queries().GetEvaluationRevision(ctx, gen.GetEvaluationRevisionParams{
		ID: id, RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationRecord{}, ErrNotFound
	}
	return evaluationRecordOf(ev), err
}

func (s *Service) Revisions(ctx context.Context, workspaceID, runID pgtype.UUID) ([]EvaluationRecord, error) {
	rows, err := s.queries().ListEvaluationRevisions(ctx, gen.ListEvaluationRevisionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	revisions := make([]EvaluationRecord, len(rows))
	for i, row := range rows {
		revisions[i] = evaluationRecordOf(row)
	}
	return revisions, nil
}

func (s *Service) SetFeedback(
	ctx context.Context, workspaceID, runID pgtype.UUID, helpful bool, comment string,
) (EvaluationRecord, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return EvaluationRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := loadCurrentEvaluation(ctx, s.queries().WithTx(tx), workspaceID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationRecord{}, ErrNotFound
	}
	if err != nil {
		return EvaluationRecord{}, err
	}
	current.RecordFeedback(helpful, comment)
	if err := saveUnlessRefused(ctx, tx, current); err != nil {
		return EvaluationRecord{}, err
	}
	return evaluationRecordOf(current.row), tx.Commit(ctx)
}
