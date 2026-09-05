package eval

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	creationFeedbackMaxRunes   = 16000
	creationFeedbackMaxSummary = 2000
	// creationFeedbackMaxItem bounds each criterion's text/reason and each
	// finding's message before the drop loops run. Without a per-item cut, the
	// tail-drop loops below remove whole criteria/findings one at a time, and a
	// single oversized free-text field (one 9,000-rune Reason) can force the
	// FAILED criterion carrying it, and the judge's own trailing warning finding,
	// to be the first things dropped for size — the two things a caller most
	// needs to see.
	creationFeedbackMaxItem = 600
)

type creationFeedbackPayload struct {
	EvaluationAvailable   bool              `json:"evaluation_available"`
	EvaluationID          string            `json:"evaluation_id"`
	Status                string            `json:"status"`
	Overall               string            `json:"overall"`
	Summary               string            `json:"summary"`
	CriterionResults      []CriterionResult `json:"criterion_results"`
	DeterministicFindings []Finding         `json:"deterministic_findings"`
	EvidenceComplete      bool              `json:"evidence_complete"`
	Truncated             bool              `json:"truncated"`
	OmittedCriteria       int               `json:"omitted_criteria"`
	OmittedFindings       int               `json:"omitted_findings"`
}

// CreationFeedback returns the small, durable report consumed by the creation
// flow. Current and view keep workspace scoping and fresh evidence availability
// in one place with the ordinary evaluation read surface.
func (s *Service) CreationFeedback(
	ctx context.Context, workspaceID, runID pgtype.UUID,
) (json.RawMessage, error) {
	ev, err := s.Current(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return json.RawMessage(`{"evaluation_available":false}`), nil
	}
	if err != nil {
		return nil, err
	}
	view, err := s.view(ctx, workspaceID, ev)
	if err != nil {
		return nil, err
	}
	return marshalCreationFeedback(view)
}

func marshalCreationFeedback(view evaluationView) (json.RawMessage, error) {
	summary, summaryTruncated := cut(view.Summary, creationFeedbackMaxSummary)
	criteria := append([]CriterionResult(nil), view.CriterionResults...)
	if criteria == nil {
		criteria = []CriterionResult{}
	}
	findings := append([]Finding(nil), view.DeterministicFindings...)
	if findings == nil {
		findings = []Finding{}
	}

	// Per-item cut runs before anything is dropped whole, so a truncated Reason
	// or Message survives even when the criterion or finding around it does not
	// make the final cut.
	itemsTruncated := false
	for i := range criteria {
		var t bool
		if criteria[i].Text, t = cut(criteria[i].Text, creationFeedbackMaxItem); t {
			itemsTruncated = true
		}
		if criteria[i].Reason, t = cut(criteria[i].Reason, creationFeedbackMaxItem); t {
			itemsTruncated = true
		}
	}
	for i := range findings {
		if msg, t := cut(findings[i].Message, creationFeedbackMaxItem); t {
			findings[i].Message = msg
			itemsTruncated = true
		}
	}

	// The tail-drop loops below remove whole items from the end, so what most
	// needs to survive goes first: a criterion already passed/met is the cheapest
	// thing to omit, and a finding below warning severity is not worth keeping
	// over one that is (04 丙 six-lane audit).
	sort.SliceStable(criteria, func(i, j int) bool {
		return criterionDropFirst(criteria[i]) < criterionDropFirst(criteria[j])
	})
	sort.SliceStable(findings, func(i, j int) bool {
		return findingDropFirst(findings[i]) < findingDropFirst(findings[j])
	})

	truncated := summaryTruncated || itemsTruncated
	evidenceComplete := view.EvidenceComplete && !summaryTruncated && !itemsTruncated
	omittedCriteria, omittedFindings := 0, 0
	encode := func() (json.RawMessage, error) {
		return json.Marshal(creationFeedbackPayload{
			EvaluationAvailable:   true,
			EvaluationID:          view.EvaluationID,
			Status:                view.Status,
			Overall:               view.Overall,
			Summary:               summary,
			CriterionResults:      criteria,
			DeterministicFindings: findings,
			EvidenceComplete:      evidenceComplete,
			Truncated:             truncated,
			OmittedCriteria:       omittedCriteria,
			OmittedFindings:       omittedFindings,
		})
	}

	blob, err := encode()
	if err != nil {
		return nil, err
	}
	if len([]rune(string(blob))) <= creationFeedbackMaxRunes {
		return blob, err
	}

	truncated = true
	evidenceComplete = false
	// Criteria first: a run typically has far more evaluated text sitting in
	// CriterionResults than in DeterministicFindings, so criteria is where an
	// oversized report is usually coming from. Emptying findings first — as this
	// used to — could burn through the judge's only warning finding chasing size
	// that criteria was responsible for the whole time, before criteria's own
	// (now-sorted) passed/met entries were ever touched.
	for len([]rune(string(blob))) > creationFeedbackMaxRunes && len(criteria) > 0 {
		criteria = criteria[:len(criteria)-1]
		omittedCriteria++
		blob, err = encode()
		if err != nil {
			return nil, err
		}
	}
	for len([]rune(string(blob))) > creationFeedbackMaxRunes && len(findings) > 0 {
		findings = findings[:len(findings)-1]
		omittedFindings++
		blob, err = encode()
		if err != nil {
			return nil, err
		}
	}

	if len([]rune(string(blob))) > creationFeedbackMaxRunes {
		return nil, errors.New("creation feedback exceeds its bound")
	}
	return blob, err
}

// criterionDropFirst ranks a criterion for the tail-drop loop: 0 keeps it near
// the front (dropped last), 1 puts it at the back (dropped first). A criterion
// already passed/met is the cheapest thing to omit — the ones still failed or
// undetermined are what a caller needs to see to fix anything.
func criterionDropFirst(c CriterionResult) int {
	if c.Result == ResultPassed {
		return 1
	}
	return 0
}

// findingDropFirst mirrors criterionDropFirst for findings: anything below
// warning severity goes to the back, dropped before a warning or an error.
func findingDropFirst(f Finding) int {
	if f.Severity == SeverityWarning || f.Severity == SeverityError {
		return 0
	}
	return 1
}
