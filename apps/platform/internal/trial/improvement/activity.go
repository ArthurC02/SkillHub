package eval

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ActivityFact struct {
	RunID          pgtype.UUID
	Classification string
	Status         string
	StatusLabel    string
	ActivityAt     time.Time
}

const evaluationNeedsAttention = "needs_attention"

func (s *Service) ActivityFacts(ctx context.Context, workspaceID pgtype.UUID) ([]ActivityFact, error) {
	rows, err := gen.New(s.Pool).ListEvaluationActivityFacts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityFact, len(rows))
	for i, row := range rows {
		facts[i] = evaluationActivityFact(row)
	}
	return facts, nil
}

func evaluationActivityFact(row gen.ListEvaluationActivityFactsRow) ActivityFact {
	classification, label, supported := classifyEvaluationActivity(Status(row.Status), Overall(row.Overall))
	activityAt := row.CreatedAt.Time
	if row.Status != string(StatusPending) && row.EvaluatedAt.Valid {
		activityAt = row.EvaluatedAt.Time
	}
	if !supported || (row.Status != string(StatusPending) && !row.EvaluatedAt.Valid) {
		classification = evaluationNeedsAttention
		label = "不支援的評估狀態"
	}
	return ActivityFact{
		RunID: row.RunID, Classification: classification, Status: row.Status,
		StatusLabel: label, ActivityAt: activityAt,
	}
}

func classifyEvaluationActivity(status Status, overall Overall) (string, string, bool) {
	switch status {
	case StatusPending:
		if overall == OverallUndetermined {
			return "in_progress", "評估中", true
		}
	case StatusFailed:
		if overall == OverallUndetermined {
			return evaluationNeedsAttention, "評估失敗", true
		}
	case StatusCompleted:
		switch overall {
		case OverallMet:
			return "recent", "符合驗收標準", true
		case OverallPartiallyMet:
			return evaluationNeedsAttention, "部分符合驗收標準", true
		case OverallNotMet:
			return evaluationNeedsAttention, "未符合驗收標準", true
		case OverallUndetermined:
			return evaluationNeedsAttention, "無法判定評估結果", true
		}
	}
	return evaluationNeedsAttention, "不支援的評估狀態", false
}
