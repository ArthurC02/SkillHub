package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func (r Reader) BelongsToWorkspace(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error) {
	return r.queries().RunInWorkspace(ctx, gen.RunInWorkspaceParams{
		ID: runID, WorkspaceID: workspaceID,
	})
}

func (r Reader) RunsInWorkspace(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	return r.queries().RunIDsInWorkspace(ctx, gen.RunIDsInWorkspaceParams{RunIds: runIDs, WorkspaceID: workspaceID})
}

type TraceRun struct {
	Status       string
	StatusReason *string
	Started      bool
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
	Terminal           bool
	Succeeded          bool
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

	EarlierQuestions []trace.Question
}

type EvaluationArtifactAbsence struct {
	Deleted int
	Expired int
}

func (r Reader) TraceRun(ctx context.Context, workspaceID, runID pgtype.UUID) (TraceRun, bool, error) {
	row, err := r.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return TraceRun{}, false, nil
	}
	if err != nil {
		return TraceRun{}, false, err
	}
	return TraceRun{
		Status: string(row.Status), StatusReason: row.StatusReason, Started: row.StartedAt.Valid,
	}, true, nil
}

func (r Reader) TraceIngestRun(ctx context.Context, runID pgtype.UUID) (TraceIngestRun, bool, error) {
	row, err := r.queries().GetRunForTraceIngest(ctx, runID)
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

func (r Reader) TraceTransitions(ctx context.Context, workspaceID, runID pgtype.UUID) ([]TraceTransition, error) {
	rows, err := r.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
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

func (r Reader) EvaluationRun(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationRun, bool, error) {
	row, err := r.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return EvaluationRun{}, false, nil
	}
	if err != nil {
		return EvaluationRun{}, false, err
	}
	snapshots, err := r.queries().GetRunSnapshots(ctx, gen.GetRunSnapshotsParams{RunID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return EvaluationRun{}, false, err
	}
	return evaluationRun(row, snapshots.RuntimeSnapshot), true, nil
}

func (r Reader) EvaluationInput(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationInput, bool, error) {
	run, found, err := r.EvaluationRun(ctx, workspaceID, runID)
	if err != nil || !found {
		return EvaluationInput{}, found, err
	}
	attempts, err := r.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return EvaluationInput{}, false, err
	}
	latestAttempt := 1
	if len(attempts) > 0 {
		latestAttempt = int(attempts[len(attempts)-1].AttemptNumber)
	}
	rows, err := r.queries().ListRunArtifactsWithLifecycle(ctx, gen.ListRunArtifactsWithLifecycleParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return EvaluationInput{}, false, err
	}
	artifacts, absent := evaluationArtifacts(rows, time.Now())
	earlier, err := r.earlierQuestions(ctx, workspaceID, runID)
	if err != nil {
		return EvaluationInput{}, false, err
	}
	return EvaluationInput{
		Run:              run,
		Artifacts:        artifacts,
		Absent:           absent,
		LatestAttempt:    latestAttempt,
		EarlierQuestions: earlier,
	}, true, nil
}

func (r Reader) earlierQuestions(ctx context.Context, workspaceID, runID pgtype.UUID) ([]trace.Question, error) {
	rounds, err := r.queries().ListEarlierQuestions(ctx, gen.ListEarlierQuestionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	var earlier []trace.Question
	for _, raw := range rounds {
		var asked []trace.Question
		if err := json.Unmarshal(raw, &asked); err != nil {
			return nil, err
		}
		earlier = append(earlier, asked...)
	}
	return earlier, nil
}

func runArtifactReadableAt(a gen.Artifact, now time.Time) bool {
	return !a.DeletedAt.Valid && !a.PurgedAt.Valid && a.ExpiresAt.Time.After(now)
}

func evaluationArtifacts(rows []gen.Artifact, now time.Time) ([]EvaluationArtifact, EvaluationArtifactAbsence) {
	artifacts := []EvaluationArtifact{}
	var absent EvaluationArtifactAbsence
	for _, row := range rows {
		switch {
		case row.DeletedAt.Valid:
			absent.Deleted++
		case !runArtifactReadableAt(row, now):
			absent.Expired++
		default:
			artifacts = append(artifacts, EvaluationArtifact{
				FileName: row.FileName, ContentType: row.ContentType,
				SizeBytes: row.SizeBytes, ContentHash: row.ContentHash,
			})
		}
	}
	return artifacts, absent
}

func evaluationRun(row gen.Run, runtimeSnapshot []byte) EvaluationRun {
	return EvaluationRun{
		ID: row.ID, WorkspaceID: row.WorkspaceID,
		SkillVersionID: row.SkillVersionID, TestCaseSnapshotID: row.TestCaseSnapshotID,
		Status: string(row.Status), StatusReason: row.StatusReason, RuntimeSnapshot: runtimeSnapshot,
		Terminal: run.IsTerminal(row.Status), Succeeded: row.Status == gen.RunStatusSucceeded,
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
