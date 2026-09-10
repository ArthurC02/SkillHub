package eval

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	creationFeedbackMaxRunes   = 16000
	creationFeedbackMaxSummary = 2000

	creationFeedbackMaxItem = 600
)

var linkPattern = regexp.MustCompile(`(?i)\b(?:https?|ftp|file|data|javascript)://[^\s<>"')]+|\bwww\.[^\s<>"')]+`)

const linkPlaceholder = "[link removed]"

func withoutLinks(s string) string {
	return linkPattern.ReplaceAllString(s, linkPlaceholder)
}

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
	summary, summaryTruncated := cut(withoutLinks(view.Summary), creationFeedbackMaxSummary)
	criteria := append([]CriterionResult(nil), view.CriterionResults...)
	if criteria == nil {
		criteria = []CriterionResult{}
	}
	findings := append([]Finding(nil), view.DeterministicFindings...)
	if findings == nil {
		findings = []Finding{}
	}

	itemsTruncated := false
	for i := range criteria {
		var t bool
		if criteria[i].Text, t = cut(criteria[i].Text, creationFeedbackMaxItem); t {
			itemsTruncated = true
		}
		if criteria[i].Reason, t = cut(withoutLinks(criteria[i].Reason), creationFeedbackMaxItem); t {
			itemsTruncated = true
		}
	}
	for i := range findings {

		msg, t := cut(withoutLinks(findings[i].Message), creationFeedbackMaxItem)
		findings[i].Message = msg
		if t {
			itemsTruncated = true
		}
	}

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

func criterionDropFirst(c CriterionResult) int {
	if c.Result == ResultPassed {
		return 1
	}
	return 0
}

func findingDropFirst(f Finding) int {
	if f.Severity == SeverityWarning || f.Severity == SeverityError {
		return 0
	}
	return 1
}
