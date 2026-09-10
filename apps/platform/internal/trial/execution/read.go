package run

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (s *Service) BelongsToWorkspace(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error) {
	return s.queries().RunInWorkspace(ctx, gen.RunInWorkspaceParams{
		ID: runID, WorkspaceID: workspaceID,
	})
}

type TraceRun struct {
	Status       string
	StatusReason *string
}

type TraceIngestRun struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	Status      string
	FinishedAt  *time.Time
}

type TraceTransition struct {
	ToStatus string
	Reason   *string
}

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

type EvaluationArtifact struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
}

type EvaluationInput struct {
	Run       EvaluationRun
	Artifacts []EvaluationArtifact

	Absent        EvaluationArtifactAbsence
	LatestAttempt int
}

type EvaluationArtifactAbsence struct {
	Deleted int
	Expired int
}

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
	rows, err := s.queries().ListReadableRunArtifacts(ctx, gen.ListReadableRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return EvaluationInput{}, false, err
	}

	absent, err := s.queries().CountUnreadableRunArtifacts(ctx, gen.CountUnreadableRunArtifactsParams{
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
	return EvaluationInput{
		Run:       run,
		Artifacts: artifacts,
		Absent: EvaluationArtifactAbsence{
			Deleted: int(absent.Deleted),
			Expired: int(absent.Expired),
		},
		LatestAttempt: latestAttempt,
	}, true, nil
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
