package eval

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	actionSuggestionProvenanceLost = "evaluation.provenance_not_recorded"
	suggestionsAppliedAttempts     = 5
)

type SkillVersionConsumer struct {
	Insert func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type versionAdded struct {
	VersionID  pgtype.UUID `json:"version_id"`
	ImprovedBy *struct {
		EvaluationID  pgtype.UUID   `json:"evaluation_id"`
		SuggestionIDs []pgtype.UUID `json:"suggestion_ids"`
	} `json:"improved_by"`
}

func (c *SkillVersionConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if event.EventType != outbox.SkillVersionAdded {
		return nil
	}
	if c.Insert == nil {
		return errors.New("suggestions-applied consumer is not configured")
	}
	var added versionAdded
	if err := json.Unmarshal(event.Payload, &added); err != nil {
		return err
	}
	if added.ImprovedBy == nil {
		return nil
	}
	_, err := c.Insert(ctx, SuggestionsAppliedArgs{
		WorkspaceID: event.WorkspaceID, SkillID: event.AggregateID, SkillVersionID: added.VersionID,
		EvaluationID: added.ImprovedBy.EvaluationID, SuggestionIDs: added.ImprovedBy.SuggestionIDs,
	}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}, MaxAttempts: suggestionsAppliedAttempts})
	return err
}

type SuggestionsAppliedArgs struct {
	WorkspaceID    pgtype.UUID   `json:"workspace_id"`
	SkillID        pgtype.UUID   `json:"skill_id"`
	SkillVersionID pgtype.UUID   `json:"skill_version_id"`
	EvaluationID   pgtype.UUID   `json:"evaluation_id"`
	SuggestionIDs  []pgtype.UUID `json:"suggestion_ids"`
}

func (SuggestionsAppliedArgs) Kind() string { return "record_suggestions_applied" }

type SuggestionsAppliedWorker struct {
	river.WorkerDefaults[SuggestionsAppliedArgs]
	Svc *Service
}

func (w *SuggestionsAppliedWorker) Work(ctx context.Context, job *river.Job[SuggestionsAppliedArgs]) error {
	a := job.Args
	err := w.Svc.RecordSuggestionsApplied(ctx, a.WorkspaceID, a.EvaluationID, a.SkillVersionID, a.SuggestionIDs)
	if err != nil && job.Attempt >= job.MaxAttempts {
		return errors.Join(err, w.Svc.auditLostProvenance(ctx, a))
	}
	return err
}

func (s *Service) RecordSuggestionsApplied(
	ctx context.Context, workspaceID, evaluationID, versionID pgtype.UUID, suggestionIDs []pgtype.UUID,
) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	e, err := loadEvaluationWithSuggestions(ctx, gen.New(tx), workspaceID, evaluationID, suggestionIDs)
	if err != nil {
		return err
	}
	e.RecordApplied(versionID, suggestionIDs)
	if _, refused := e.Refusal(); refused {
		return nil
	}
	if err := saveEvaluation(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) auditLostProvenance(ctx context.Context, a SuggestionsAppliedArgs) error {
	return audit.Log(ctx, s.Pool, audit.Event{
		Workspace:    a.WorkspaceID,
		Action:       actionSuggestionProvenanceLost,
		ResourceType: audit.ResourceVersion,
		ResourceID:   a.SkillVersionID,
		Metadata: map[string]any{
			"evaluation_id":  pgconv.UUIDString(a.EvaluationID),
			"skill_id":       pgconv.UUIDString(a.SkillID),
			"suggestions":    len(a.SuggestionIDs),
			"version_exists": true,
		},
	})
}
