package eval

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnyFeedbackItemCutAtItsLimitMarksTheReportTruncatedAndIncomplete(t *testing.T) {
	short := "fine"
	for _, tc := range []struct {
		name          string
		text, reason  string
		message       string
		wantTruncated bool
	}{
		{"every item fits", short, short, short, false},
		{"a criterion text exactly at the limit", strings.Repeat("a", creationFeedbackMaxItem), short, short, false},
		{"a criterion text one past the limit", strings.Repeat("a", creationFeedbackMaxItem+1), short, short, true},
		{"a criterion reason one past the limit", short, strings.Repeat("b", creationFeedbackMaxItem+1), short, true},
		{"a finding message one past the limit", short, short, strings.Repeat("c", creationFeedbackMaxItem+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := marshalCreationFeedback(evaluationView{
				EvaluationID: "e1", Status: StatusCompleted, Overall: string(OverallNotMet),
				Summary: short, EvidenceComplete: true,
				CriterionResults:      []CriterionResult{{CriterionID: "c1", Text: tc.text, Reason: tc.reason}},
				DeterministicFindings: []Finding{{Category: CategoryCost, Severity: SeverityWarning, Message: tc.message}},
			})
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Truncated        bool `json:"truncated"`
				EvidenceComplete bool `json:"evidence_complete"`
			}
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Truncated != tc.wantTruncated || payload.EvidenceComplete == tc.wantTruncated {
				t.Errorf("truncated=%v evidence_complete=%v, want truncated=%v and complete=%v",
					payload.Truncated, payload.EvidenceComplete, tc.wantTruncated, !tc.wantTruncated)
			}
		})
	}
}
