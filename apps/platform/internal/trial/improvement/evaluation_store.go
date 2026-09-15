package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func loadEvaluation(ctx context.Context, q *gen.Queries, workspaceID, id pgtype.UUID) (*Evaluation, error) {
	row, err := q.LockEvaluation(ctx, gen.LockEvaluationParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return &Evaluation{row: row}, nil
}

func loadCurrentEvaluation(ctx context.Context, q *gen.Queries, workspaceID, runID pgtype.UUID) (*Evaluation, error) {
	row, err := q.LockCurrentEvaluation(ctx, gen.LockCurrentEvaluationParams{RunID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return &Evaluation{row: row}, nil
}

func loadEvaluationOfSuggestion(
	ctx context.Context, q *gen.Queries, workspaceID, suggestionID pgtype.UUID,
) (*Evaluation, error) {
	owner, err := q.GetEvaluationSuggestion(ctx, gen.GetEvaluationSuggestionParams{
		ID: suggestionID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	e, err := loadEvaluation(ctx, q, workspaceID, owner.EvaluationID)
	if err != nil {
		return nil, err
	}
	suggestion, err := q.LockEvaluationSuggestion(ctx, gen.LockEvaluationSuggestionParams{
		ID: suggestionID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{suggestion.ID: suggestion}
	return e, nil
}

func loadEvaluationWithSuggestions(
	ctx context.Context, q *gen.Queries, workspaceID, evaluationID pgtype.UUID, suggestionIDs []pgtype.UUID,
) (*Evaluation, error) {
	e, err := loadEvaluation(ctx, q, workspaceID, evaluationID)
	if err != nil {
		return nil, err
	}
	e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{}
	for _, id := range suggestionIDs {
		suggestion, err := q.LockEvaluationSuggestion(ctx, gen.LockEvaluationSuggestionParams{
			ID: id, WorkspaceID: workspaceID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if suggestion.EvaluationID == e.row.ID {
			e.suggestions[id] = suggestion
		}
	}
	applications, err := q.ListEvaluationSuggestionApplications(ctx, gen.ListEvaluationSuggestionApplicationsParams{
		EvaluationID: evaluationID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	e.applied = map[pgtype.UUID]map[pgtype.UUID]struct{}{}
	for _, application := range applications {
		if e.applied[application.SuggestionID] == nil {
			e.applied[application.SuggestionID] = map[pgtype.UUID]struct{}{}
		}
		e.applied[application.SuggestionID][application.SkillVersionID] = struct{}{}
	}
	return e, nil
}

func saveUnlessRefused(ctx context.Context, tx pgx.Tx, e *Evaluation) error {
	if reason, refused := e.Refusal(); refused {
		return reason.err()
	}
	return saveEvaluation(ctx, tx, e)
}

func saveEvaluation(ctx context.Context, tx pgx.Tx, e *Evaluation) error {
	q := gen.New(tx)
	for _, event := range e.events {
		if err := writeEvaluationEvent(ctx, q, e, event); err != nil {
			return err
		}
		if err := outbox.Insert(ctx, tx, outbox.NewEvent{
			EventType: event.eventType(), EventVersion: outbox.EventVersion1,
			CorrelationID: e.row.RunID, WorkspaceID: e.row.WorkspaceID,
			AggregateType: outbox.AggregateEvaluation, AggregateID: e.row.ID, Payload: event,
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeEvaluationEvent(ctx context.Context, q *gen.Queries, e *Evaluation, event Event) error {
	var err error
	switch event := event.(type) {
	case EvaluationStarted:
		e.row, err = q.CreateEvaluation(ctx, gen.CreateEvaluationParams{
			WorkspaceID: e.row.WorkspaceID, RunID: e.row.RunID,
			JudgeModel: event.JudgeModel, JudgePromptVersion: event.JudgePromptVersion,
			RubricVersion: event.RubricVersion,
		})
	case EvaluationSuperseded:
		_, err = q.SupersedeCurrentEvaluation(ctx, gen.SupersedeCurrentEvaluationParams{
			RunID: e.row.RunID, WorkspaceID: e.row.WorkspaceID,
		})
	case EvaluationCompleted:
		e.row, err = writeCompletion(ctx, q, e)
	case EvaluationFailed:
		e.row, err = writeFailure(ctx, q, e)
	case FeedbackRecorded:
		e.row, err = q.SetEvaluationFeedback(ctx, gen.SetEvaluationFeedbackParams{
			ID: e.row.ID, WorkspaceID: e.row.WorkspaceID,
			FeedbackHelpful: e.row.FeedbackHelpful, FeedbackComment: e.row.FeedbackComment,
		})
	case SuggestionDecided:
		var decided gen.EvaluationSuggestion
		if decided, err = q.DecideSuggestion(ctx, gen.DecideSuggestionParams{
			Decision: string(event.Decision), ID: event.SuggestionID, WorkspaceID: e.row.WorkspaceID,
		}); err == nil {
			e.suggestions[event.SuggestionID] = decided
		}
	case SuggestionsApplied:
		_, err = q.MarkSuggestionsApplied(ctx, gen.MarkSuggestionsAppliedParams{
			SkillVersionID: event.SkillVersionID, Ids: event.SuggestionIDs, WorkspaceID: e.row.WorkspaceID,
			EvaluationID: e.row.ID,
		})
	default:
		err = fmt.Errorf("evaluation event %T has nothing to write", event)
	}
	return err
}

func writeCompletion(ctx context.Context, q *gen.Queries, e *Evaluation) (gen.Evaluation, error) {
	v := e.verdict
	results, err := json.Marshal(v.results)
	if err != nil {
		return e.row, err
	}
	findings, err := json.Marshal(nonNilFindings(v.findings))
	if err != nil {
		return e.row, err
	}
	row, err := q.CompleteEvaluation(ctx, gen.CompleteEvaluationParams{
		ID: e.row.ID, WorkspaceID: e.row.WorkspaceID,
		Overall:               string(v.overall),
		Summary:               strPtr(v.summary),
		CriterionResults:      results,
		DeterministicFindings: findings,
		JudgeModel:            strPtr(v.model),
		JudgePromptVersion:    strPtr(v.promptVersion),
		RubricVersion:         strPtr(v.rubricVersion),
		EvidenceComplete:      v.evidenceComplete,
		CostUsd:               numeric(v.costUSD),
		CostSource:            costSource(v.costUSD),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return e.row, errEvaluationSettled
	}
	return row, err
}

func writeFailure(ctx context.Context, q *gen.Queries, e *Evaluation) (gen.Evaluation, error) {
	f := e.failure
	findings, err := json.Marshal(nonNilFindings(f.findings))
	if err != nil {
		return e.row, err
	}
	row, err := q.FailEvaluation(ctx, gen.FailEvaluationParams{
		ID: e.row.ID, WorkspaceID: e.row.WorkspaceID,
		Summary:               strPtr(f.summary),
		DeterministicFindings: findings,
		EvidenceComplete:      f.evidenceComplete,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return e.row, errEvaluationSettled
	}
	return row, err
}

func supersedeCurrent(ctx context.Context, tx pgx.Tx, q *gen.Queries, workspaceID, runID pgtype.UUID) error {
	current, err := loadCurrentEvaluation(ctx, q, workspaceID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	current.Supersede()
	return saveUnlessRefused(ctx, tx, current)
}

func settleEvaluation(
	ctx context.Context, tx pgx.Tx, q *gen.Queries, ev gen.Evaluation, settle func(*Evaluation),
) error {
	e, err := loadEvaluation(ctx, q, ev.WorkspaceID, ev.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errEvaluationSettled
	}
	if err != nil {
		return err
	}
	settle(e)
	return saveUnlessRefused(ctx, tx, e)
}
