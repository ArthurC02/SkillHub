package run

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// BelongsToWorkspace answers membership without exposing Run's generated query
// contract to consumers.
func (s *Service) BelongsToWorkspace(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error) {
	return s.queries().RunInWorkspace(ctx, gen.RunInWorkspaceParams{
		ID: runID, WorkspaceID: workspaceID,
	})
}

// TraceRun is the run state Trace may show without exposing sqlc's Run row.
type TraceRun struct {
	Status       string
	StatusReason *string
}

// TraceIngestRun is the trusted scope and lifecycle state resolved from a
// signed ingestion grant. The workspace never comes from the event body.
type TraceIngestRun struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	Status      string
	FinishedAt  *time.Time
}

// TraceTransition is one authoritative lifecycle step rendered by Trace.
type TraceTransition struct {
	ToStatus string
	Reason   *string
}

// EvaluationRun is the Run-owned fact shape used by Evaluation reads.
type EvaluationRun struct {
	ID                 pgtype.UUID
	WorkspaceID        pgtype.UUID
	SkillVersionID     pgtype.UUID
	TestCaseSnapshotID pgtype.UUID
	Status             string
	StatusReason       *string
	RuntimeSnapshot    []byte
	StartedAt          *time.Time
	FinishedAt         *time.Time
	FailureClass       *string
}

// EvaluationArtifact is one live output-manifest row used as evidence.
type EvaluationArtifact struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
}

// EvaluationInput is the Run-owned aggregate needed to evaluate one attempt.
type EvaluationInput struct {
	Run           EvaluationRun
	Artifacts     []EvaluationArtifact
	LatestAttempt int
}

// TraceRun returns a workspace-scoped run fact for Trace.
func (s *Service) TraceRun(ctx context.Context, workspaceID, runID pgtype.UUID) (TraceRun, bool, error) {
	row, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return TraceRun{}, false, nil
	}
	if err != nil {
		return TraceRun{}, false, err
	}
	return TraceRun{Status: string(row.Status), StatusReason: row.StatusReason}, true, nil
}

// TraceIngestRun resolves the owner-controlled workspace and state named by a
// signed run grant.
func (s *Service) TraceIngestRun(ctx context.Context, runID pgtype.UUID) (TraceIngestRun, bool, error) {
	row, err := s.queries().GetRunForTraceIngest(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TraceIngestRun{}, false, nil
	}
	if err != nil {
		return TraceIngestRun{}, false, err
	}
	var finishedAt *time.Time
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time
		finishedAt = &finished
	}
	return TraceIngestRun{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Status: string(row.Status), FinishedAt: finishedAt,
	}, true, nil
}

// TraceTransitions returns authoritative lifecycle steps in occurrence order.
func (s *Service) TraceTransitions(ctx context.Context, workspaceID, runID pgtype.UUID) ([]TraceTransition, error) {
	rows, err := s.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TraceTransition, len(rows))
	for i, row := range rows {
		out[i] = TraceTransition{ToStatus: string(row.ToStatus), Reason: row.Reason}
	}
	return out, nil
}

// EvaluationRun returns one workspace-scoped Run fact without exposing sqlc.
func (s *Service) EvaluationRun(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationRun, bool, error) {
	row, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationRun{}, false, nil
	}
	if err != nil {
		return EvaluationRun{}, false, err
	}
	return evaluationRun(row), true, nil
}

// EvaluationInput returns Run facts, the latest attempt number (default 1),
// and live output evidence under the same workspace scope.
func (s *Service) EvaluationInput(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationInput, bool, error) {
	run, found, err := s.EvaluationRun(ctx, workspaceID, runID)
	if err != nil || !found {
		return EvaluationInput{}, found, err
	}
	attempts, err := s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return EvaluationInput{}, false, err
	}
	latestAttempt := 1
	if len(attempts) > 0 {
		latestAttempt = int(attempts[len(attempts)-1].AttemptNumber)
	}
	rows, err := s.queries().ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return EvaluationInput{}, false, err
	}
	artifacts := make([]EvaluationArtifact, len(rows))
	for i, row := range rows {
		artifacts[i] = EvaluationArtifact{
			FileName: row.FileName, ContentType: row.ContentType,
			SizeBytes: row.SizeBytes, ContentHash: row.ContentHash,
		}
	}
	return EvaluationInput{Run: run, Artifacts: artifacts, LatestAttempt: latestAttempt}, true, nil
}

func evaluationRun(row gen.Run) EvaluationRun {
	return EvaluationRun{
		ID: row.ID, WorkspaceID: row.WorkspaceID,
		SkillVersionID: row.SkillVersionID, TestCaseSnapshotID: row.TestCaseSnapshotID,
		Status: string(row.Status), StatusReason: row.StatusReason, RuntimeSnapshot: row.RuntimeSnapshot,
		StartedAt: timePtr(row.StartedAt), FinishedAt: timePtr(row.FinishedAt), FailureClass: row.FailureClass,
	}
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time
	return &at
}
