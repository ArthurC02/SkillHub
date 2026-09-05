package eval

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalCreationFeedbackKeepsFailedCriterionReasonAndEvidence(t *testing.T) {
	view := evaluationView{
		EvaluationID:     "evaluation-1",
		Status:           StatusCompleted,
		Overall:          OverallNotMet,
		Summary:          "The file was not produced.",
		EvidenceComplete: true,
		CriterionResults: []CriterionResult{{
			CriterionID: "criterion-1",
			Text:        "an output file is produced",
			Result:      ResultFailed,
			Reason:      "No output file was found.",
			Evidence: []EvidenceRef{{
				Kind:      KindArtifact,
				Match:     MatchExact,
				Excerpt:   "output.xlsx",
				Available: true,
			}},
		}},
	}

	got, err := marshalCreationFeedback(view)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	var results []CriterionResult
	if err := json.Unmarshal(payload["criterion_results"], &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Reason != "No output file was found." {
		t.Fatalf("failed criterion reason was not preserved: %+v", results)
	}
	if len(results[0].Evidence) != 1 || results[0].Evidence[0].Excerpt != "output.xlsx" {
		t.Fatalf("failed criterion evidence was not preserved: %+v", results[0].Evidence)
	}
	for _, key := range []string{"judge_model", "cost", "feedback", "run_id", "raw_trace", "artifact"} {
		if _, ok := payload[key]; ok {
			t.Errorf("disallowed field %q present", key)
		}
	}
}

func TestMarshalCreationFeedbackBoundsLargeReportAndMarksOmissions(t *testing.T) {
	large := strings.Repeat("x", 9000)
	view := evaluationView{
		EvaluationID:          "evaluation-2",
		Status:                StatusFailed,
		Overall:               OverallUndetermined,
		Summary:               strings.Repeat("摘要", 1500),
		EvidenceComplete:      true,
		CriterionResults:      []CriterionResult{{CriterionID: "criterion-1", Text: large, Result: ResultFailed, Reason: large}, {CriterionID: "criterion-2", Text: large, Result: ResultFailed, Reason: large}},
		DeterministicFindings: []Finding{{Category: CategoryExecution, Severity: SeverityError, Message: large}, {Category: CategoryCost, Severity: SeverityWarning, Message: large}},
	}

	got, err := marshalCreationFeedback(view)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(string(got))) > creationFeedbackMaxRunes {
		t.Fatalf("report has %d runes, want <= %d", len([]rune(string(got))), creationFeedbackMaxRunes)
	}
	var payload struct {
		Status           string `json:"status"`
		Overall          string `json:"overall"`
		Summary          string `json:"summary"`
		EvidenceComplete bool   `json:"evidence_complete"`
		Truncated        bool   `json:"truncated"`
		OmittedCriteria  int    `json:"omitted_criteria"`
		OmittedFindings  int    `json:"omitted_findings"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Status != StatusFailed || payload.Overall != OverallUndetermined {
		t.Fatalf("verdict changed: %+v", payload)
	}
	if len([]rune(payload.Summary)) > creationFeedbackMaxSummary {
		t.Fatalf("summary has %d runes, want <= %d", len([]rune(payload.Summary)), creationFeedbackMaxSummary)
	}
	if !payload.Truncated || payload.EvidenceComplete || payload.OmittedCriteria+payload.OmittedFindings == 0 {
		t.Fatalf("missing truncation markers: %+v", payload)
	}
}
