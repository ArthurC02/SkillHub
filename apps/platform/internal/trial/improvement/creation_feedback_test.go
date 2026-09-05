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
	// Enough criteria that even after each one's Text/Reason is cut to
	// creationFeedbackMaxItem, the payload still exceeds creationFeedbackMaxRunes
	// and the whole-item drop loop still has to run.
	var criteria []CriterionResult
	for i := 0; i < 15; i++ {
		criteria = append(criteria, CriterionResult{
			CriterionID: "criterion", Text: large, Result: ResultFailed, Reason: large,
		})
	}
	view := evaluationView{
		EvaluationID:          "evaluation-2",
		Status:                StatusFailed,
		Overall:               OverallUndetermined,
		Summary:               strings.Repeat("摘要", 1500),
		EvidenceComplete:      true,
		CriterionResults:      criteria,
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

// TestMarshalCreationFeedbackPerItemCutProtectsFailedCriterionAndWarning pins
// the actual seam bug: a per-item cut on CriterionResult.Reason and
// Finding.Message, applied before anything is dropped whole, plus ordering
// that drops passed/met criteria and non-warning/error findings first. Without
// it, a single oversized free-text field forced exactly the content a caller
// most needs — the failed criterion's reason, the judge's warning — to be the
// first things lost to the byte budget.
//
// Two criteria carry a full, uncut 9,000-rune reason each (an undetermined one
// and, positioned last the way a real evaluation would append it, the failed
// one this test is about): cut to creationFeedbackMaxItem apiece they fit
// comfortably, but full-size together they are what actually forces the
// criteria drop loop to reach past the merely-`passed` filler and into
// protected territory — which is exactly the scenario that used to claim the
// failed criterion first, because it was the one written last.
func TestMarshalCreationFeedbackPerItemCutProtectsFailedCriterionAndWarning(t *testing.T) {
	large := strings.Repeat("y", 9000)

	criteria := []CriterionResult{
		{CriterionID: "criterion-passed", Text: "a short passed criterion", Result: ResultPassed, Reason: "ok"},
		{CriterionID: "criterion-undetermined", Text: "a filler criterion", Result: ResultUndetermined, Reason: large},
	}
	// Padding: enough passed filler criteria that the report is oversized before
	// any per-item cut is considered, so the criteria drop loop actually runs.
	for i := 0; i < 14; i++ {
		criteria = append(criteria, CriterionResult{
			CriterionID: "criterion-filler", Text: large, Result: ResultPassed, Reason: large,
		})
	}
	criteria = append(criteria, CriterionResult{
		CriterionID: "criterion-failed", Text: "the output must exist", Result: ResultFailed, Reason: large,
	})

	view := evaluationView{
		EvaluationID:     "evaluation-3",
		Status:           StatusCompleted,
		Overall:          OverallNotMet,
		Summary:          "short summary",
		EvidenceComplete: true,
		CriterionResults: criteria,
		DeterministicFindings: []Finding{
			{Category: CategoryCost, Severity: SeverityWarning, Message: "the run went over budget"},
		},
	}

	got, err := marshalCreationFeedback(view)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(string(got))) > creationFeedbackMaxRunes {
		t.Fatalf("report has %d runes, want <= %d", len([]rune(string(got))), creationFeedbackMaxRunes)
	}
	var payload struct {
		Truncated             bool              `json:"truncated"`
		OmittedCriteria       int               `json:"omitted_criteria"`
		CriterionResults      []CriterionResult `json:"criterion_results"`
		DeterministicFindings []Finding         `json:"deterministic_findings"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !payload.Truncated || payload.OmittedCriteria == 0 {
		t.Fatalf("expected the criteria drop loop to actually run: %+v", payload)
	}

	var failed *CriterionResult
	for i := range payload.CriterionResults {
		if payload.CriterionResults[i].CriterionID == "criterion-failed" {
			failed = &payload.CriterionResults[i]
		}
	}
	if failed == nil {
		t.Fatal("the failed criterion was dropped; a passed/undetermined filler should have gone first")
	}
	if failed.Reason == "" || len([]rune(failed.Reason)) > creationFeedbackMaxItem {
		t.Errorf("failed criterion reason = %d runes, want a non-empty cut reason (<= %d)",
			len([]rune(failed.Reason)), creationFeedbackMaxItem)
	}

	if len(payload.DeterministicFindings) != 1 || payload.DeterministicFindings[0].Severity != SeverityWarning {
		t.Fatalf("the trailing warning finding was dropped: %+v", payload.DeterministicFindings)
	}
}
