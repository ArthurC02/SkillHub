package eval

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type EvaluationRecord struct {
	ID                    pgtype.UUID
	WorkspaceID           pgtype.UUID
	RunID                 pgtype.UUID
	Overall               string
	Summary               *string
	CriterionResults      []byte
	JudgeModel            *string
	FeedbackHelpful       *bool
	FeedbackComment       *string
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	Status                string
	JudgePromptVersion    *string
	RubricVersion         *string
	EvidenceComplete      bool
	DeterministicFindings []byte
	CostUSD               pgtype.Numeric
	CostSource            *string
	CostIsLowerBound      bool
	EvaluatedAt           pgtype.Timestamptz
	SupersededAt          pgtype.Timestamptz
}

func evaluationRecordOf(row gen.Evaluation) EvaluationRecord {
	return EvaluationRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, RunID: row.RunID, Overall: row.Overall,
		Summary: row.Summary, CriterionResults: row.CriterionResults, JudgeModel: row.JudgeModel,
		FeedbackHelpful: row.FeedbackHelpful, FeedbackComment: row.FeedbackComment,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Status: row.Status,
		JudgePromptVersion: row.JudgePromptVersion, RubricVersion: row.RubricVersion,
		EvidenceComplete: row.EvidenceComplete, DeterministicFindings: row.DeterministicFindings,
		CostUSD: row.CostUsd, CostSource: row.CostSource, CostIsLowerBound: row.CostIsLowerBound,
		EvaluatedAt: row.EvaluatedAt, SupersededAt: row.SupersededAt,
	}
}
