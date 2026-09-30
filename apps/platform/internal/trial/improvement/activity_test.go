package eval

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestEvaluationActivityDecisionTable(t *testing.T) {
	tests := []struct {
		name      string
		status    Status
		overall   Overall
		want      string
		supported bool
	}{
		{"pending", StatusPending, OverallUndetermined, "in_progress", true},
		{"failed", StatusFailed, OverallUndetermined, "needs_attention", true},
		{"met", StatusCompleted, OverallMet, "recent", true},
		{"partially met", StatusCompleted, OverallPartiallyMet, "needs_attention", true},
		{"not met", StatusCompleted, OverallNotMet, "needs_attention", true},
		{"undetermined", StatusCompleted, OverallUndetermined, "needs_attention", true},
		{"illegal pending result", StatusPending, OverallMet, "needs_attention", false},
		{"future state", Status("future"), OverallUndetermined, "needs_attention", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, label, supported := classifyEvaluationActivity(test.status, test.overall)
			if got != test.want || supported != test.supported || label == "" {
				t.Fatalf("classification = %q, supported = %t, label = %q", got, supported, label)
			}
		})
	}
}

func TestEvaluationActivityUsesStatusSpecificOwnerTime(t *testing.T) {
	created := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	evaluated := created.Add(time.Hour)
	for _, test := range []struct {
		name      string
		status    Status
		evaluated pgtype.Timestamptz
		want      time.Time
		label     string
	}{
		{"pending uses creation", StatusPending, pgtype.Timestamptz{}, created, "評估中"},
		{"completed uses evaluation", StatusCompleted, pgtype.Timestamptz{Time: evaluated, Valid: true}, evaluated, "無法判定評估結果"},
		{"missing terminal time is unsupported", StatusFailed, pgtype.Timestamptz{}, created, "不支援的評估狀態"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact := evaluationActivityFact(gen.ListEvaluationActivityFactsRow{
				Status: string(test.status), Overall: string(OverallUndetermined),
				CreatedAt: pgtype.Timestamptz{Time: created, Valid: true}, EvaluatedAt: test.evaluated,
			})
			if !fact.ActivityAt.Equal(test.want) || fact.StatusLabel != test.label {
				t.Fatalf("fact = %+v, want time %s and label %q", fact, test.want, test.label)
			}
		})
	}
}
