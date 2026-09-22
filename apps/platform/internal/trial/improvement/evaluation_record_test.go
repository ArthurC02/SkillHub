package eval

import (
	"bytes"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestEvaluationRecordOfPreservesThePersistentEvaluation(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	runID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	value := "value"
	helpful := true
	at := pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}
	var cost pgtype.Numeric
	if err := cost.Scan("0.0123"); err != nil {
		t.Fatal(err)
	}
	row := gen.Evaluation{
		ID: id, WorkspaceID: workspaceID, RunID: runID, Overall: "pass", Summary: &value,
		CriterionResults: []byte("criteria"), JudgeModel: &value, FeedbackHelpful: &helpful,
		FeedbackComment: &value, CreatedAt: at, UpdatedAt: at, Status: "completed",
		JudgePromptVersion: &value, RubricVersion: &value, EvidenceComplete: true,
		DeterministicFindings: []byte("findings"), CostUsd: cost, CostSource: &value,
		CostIsLowerBound: true, EvaluatedAt: at, SupersededAt: at,
	}

	got := evaluationRecordOf(row)
	if got.ID != id || got.WorkspaceID != workspaceID || got.RunID != runID ||
		got.Overall != "pass" || got.Summary != &value || got.JudgeModel != &value ||
		got.FeedbackHelpful != &helpful || got.FeedbackComment != &value ||
		got.CreatedAt != at || got.UpdatedAt != at || got.Status != "completed" ||
		got.JudgePromptVersion != &value || got.RubricVersion != &value || !got.EvidenceComplete ||
		!bytes.Equal(got.CriterionResults, row.CriterionResults) ||
		!bytes.Equal(got.DeterministicFindings, row.DeterministicFindings) ||
		got.CostUSD != cost || got.CostSource != &value || !got.CostIsLowerBound ||
		got.EvaluatedAt != at || got.SupersededAt != at {
		t.Fatalf("evaluationRecordOf() = %+v, want all Evaluation fields preserved", got)
	}
}
