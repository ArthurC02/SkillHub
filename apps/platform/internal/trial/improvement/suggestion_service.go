package eval

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type AppliedSuggestion struct {
	EvaluationID pgtype.UUID
	ReflectionID pgtype.UUID
	Category     string
	TargetPath   string
}

type Suggestion struct {
	ID                    pgtype.UUID
	WorkspaceID           pgtype.UUID
	EvaluationID          pgtype.UUID
	ReflectionID          pgtype.UUID
	Category              string
	Problem               string
	Evidence              []byte
	TargetPath            string
	ProposedContent       string
	ExpectedImpact        string
	Decision              string
	DecidedAt             pgtype.Timestamptz
	AppliedSkillVersionID pgtype.UUID
	CreatedAt             pgtype.Timestamptz
}

func (s *Service) AppliedSuggestions(ctx context.Context, workspaceID, versionID pgtype.UUID) ([]AppliedSuggestion, error) {
	rows, err := gen.New(s.Pool).ListSuggestionsAppliedToVersion(ctx, gen.ListSuggestionsAppliedToVersionParams{
		AppliedSkillVersionID: versionID,
		WorkspaceID:           workspaceID,
	})
	if err != nil {
		return nil, err
	}
	result := make([]AppliedSuggestion, len(rows))
	for i, row := range rows {
		result[i] = AppliedSuggestion{
			EvaluationID: row.EvaluationID,
			ReflectionID: row.ReflectionID,
			Category:     row.Category,
			TargetPath:   row.TargetPath,
		}
	}
	return result, nil
}

func (s *Service) Decide(
	ctx context.Context, workspaceID, suggestionID pgtype.UUID, to Decision,
) (Suggestion, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Suggestion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries().WithTx(tx)
	owner, err := q.GetEvaluationSuggestion(ctx, gen.GetEvaluationSuggestionParams{ID: suggestionID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Suggestion{}, ErrNotFound
	}
	if err != nil {
		return Suggestion{}, err
	}
	decided, err := decideOwned(ctx, tx, q, owner, to)
	if errors.Is(err, pgx.ErrNoRows) {
		return Suggestion{}, ErrNotFound
	}
	if err != nil {
		return Suggestion{}, err
	}
	return suggestionOf(decided), tx.Commit(ctx)
}

func decideOwned(ctx context.Context, tx pgx.Tx, q *gen.Queries, owner gen.EvaluationSuggestion, to Decision) (gen.EvaluationSuggestion, error) {
	ids := []pgtype.UUID{owner.ID}
	if owner.ReflectionID.Valid {
		r, err := loadReflectionWithSuggestions(ctx, q, owner.WorkspaceID, owner.ReflectionID, ids)
		if err != nil {
			return gen.EvaluationSuggestion{}, err
		}
		r.Decide(owner.ID, to)
		return r.suggestions[owner.ID], saveReflectionUnlessRefused(ctx, tx, r)
	}
	e, err := loadEvaluationOfSuggestion(ctx, q, owner.WorkspaceID, owner.ID)
	if err != nil {
		return gen.EvaluationSuggestion{}, err
	}
	e.Decide(owner.ID, to)
	return e.suggestions[owner.ID], saveUnlessRefused(ctx, tx, e)
}

func (s *Service) Suggestions(
	ctx context.Context, workspaceID, evaluationID pgtype.UUID,
) ([]Suggestion, error) {
	rows, err := s.queries().ListEvaluationSuggestions(ctx,
		gen.ListEvaluationSuggestionsParams{EvaluationID: evaluationID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	result := make([]Suggestion, len(rows))
	for i, row := range rows {
		result[i] = suggestionOf(row)
	}
	return result, nil
}

func suggestionOf(row gen.EvaluationSuggestion) Suggestion {
	return Suggestion{
		ID: row.ID, WorkspaceID: row.WorkspaceID, EvaluationID: row.EvaluationID, ReflectionID: row.ReflectionID,
		Category: row.Category, Problem: row.Problem, Evidence: row.Evidence,
		TargetPath: row.TargetPath, ProposedContent: row.ProposedContent,
		ExpectedImpact: row.ExpectedImpact, Decision: row.Decision,
		DecidedAt: row.DecidedAt, AppliedSkillVersionID: row.AppliedSkillVersionID,
		CreatedAt: row.CreatedAt,
	}
}
