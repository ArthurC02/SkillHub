package eval

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	creationFeedbackMaxRunes   = 16000
	creationFeedbackMaxSummary = 2000
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

	truncated := summaryTruncated
	evidenceComplete := view.EvidenceComplete && !summaryTruncated
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
	for len([]rune(string(blob))) > creationFeedbackMaxRunes && len(findings) > 0 {
		findings = findings[:len(findings)-1]
		omittedFindings++
		blob, err = encode()
		if err != nil {
			return nil, err
		}
	}
	for len([]rune(string(blob))) > creationFeedbackMaxRunes && len(criteria) > 0 {
		criteria = criteria[:len(criteria)-1]
		omittedCriteria++
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
