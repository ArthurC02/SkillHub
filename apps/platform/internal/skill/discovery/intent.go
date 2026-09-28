package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

const (
	maxIntentKeywords     = 8   // one-number: intentMaxKeywords
	maxIntentKeywordRunes = 128 // one-number: intentMaxKeywordRunes
	// budget-over: intent.TIMEOUT_SECONDS
	intentDeadline = 13 * time.Second
)

var IntentBudget = modelbudget.Endpoint{Kind: "analyze-intent", Deadline: intentDeadline}

type IntentAnalyzer interface {
	AnalyzeIntent(context.Context, string, time.Duration) (*IntentAnalysis, error)
}

type IntentAnalysis struct {
	Valid          bool
	Interpretation SearchInterpretation
	Usage          *ModelUsage
}

type SearchInterpretation struct {
	Status         string             `json:"status"`
	Intent         map[string]*string `json:"intent"`
	Keywords       []string           `json:"keywords"`
	Filters        map[string]string  `json:"filters"`
	Model          string             `json:"model,omitempty"`
	PromptVersion  string             `json:"prompt_version,omitempty"`
	FallbackReason string             `json:"fallback_reason,omitempty"`
}

const (
	interpretationSkipped   = "skipped"
	interpretationFallback  = "fallback"
	interpretationAnalyzed  = "analyzed"
	interpretationCorrected = "corrected"
)

const (
	fallbackUnavailable     = "unavailable"
	fallbackTimeout         = "timeout"
	fallbackInvalidResponse = "invalid_response"
)

const (
	intentInput       = "input"
	intentOutput      = "output"
	intentTools       = "tools"
	intentData        = "data"
	intentEnvironment = "environment"
)

var intentFields = []string{intentInput, intentOutput, intentTools, intentData, intentEnvironment}

func emptyInterpretation(status string, filters searchFilters) SearchInterpretation {
	return SearchInterpretation{
		Status:   status,
		Intent:   map[string]*string{intentInput: nil, intentOutput: nil, intentTools: nil, intentData: nil, intentEnvironment: nil},
		Keywords: []string{}, Filters: filters.values(),
	}
}

type interpretationSource int

const (
	extractedByModel interpretationSource = iota
	correctedByUser
)

func (i SearchInterpretation) validate(query string, source interpretationSource) error {
	extracted := source == extractedByModel
	if len(i.Intent) != len(intentFields) || i.Keywords == nil || i.Filters == nil {
		return errors.New("intent, keywords and filters are required")
	}
	if err := i.validateIntentFields(query, source); err != nil {
		return err
	}
	if err := i.validateKeywords(); err != nil {
		return err
	}
	for key := range i.Filters {
		switch key {
		case filterScript, filterValidation, filterAgent, filterTier, filterCategory:
		default:
			return errors.New("unsupported search filter")
		}
	}
	if !extracted && utf8.RuneCountInString(i.retrievalQuery(query)) > maxQueryRunes {
		return errors.New("combined intent and keywords must not exceed 2000 characters")
	}
	_, err := parseFilterValues(i.Filters)
	return err
}

func (i SearchInterpretation) validateIntentFields(query string, source interpretationSource) error {
	for _, field := range intentFields {
		value, exists := i.Intent[field]
		if !exists {
			return errors.New("all five intent fields are required")
		}
		if value != nil && (strings.TrimSpace(*value) == "" || utf8.RuneCountInString(*value) > maxQueryRunes ||
			(source == extractedByModel && !strings.Contains(query, *value))) {
			return errors.New("invalid intent field")
		}
	}
	return nil
}

func (i SearchInterpretation) validateKeywords() error {
	if len(i.Keywords) > maxIntentKeywords {
		return errors.New("too many search keywords")
	}
	for _, keyword := range i.Keywords {
		if strings.TrimSpace(keyword) == "" || utf8.RuneCountInString(keyword) > maxIntentKeywordRunes {
			return errors.New("invalid search keyword")
		}
	}
	return nil
}

func (i SearchInterpretation) retrievalQuery(original string) string {
	var terms []string
	if i.Status == interpretationCorrected {
		for _, field := range intentFields {
			if value := i.Intent[field]; value != nil && !slices.Contains(terms, *value) {
				terms = append(terms, *value)
			}
		}
	}
	for _, keyword := range i.Keywords {
		if !slices.Contains(terms, keyword) {
			terms = append(terms, keyword)
		}
	}
	if len(terms) == 0 {
		return original
	}
	return strings.Join(terms, " ")
}

func (s *Service) interpret(ctx context.Context, query string, filters searchFilters, purpose searchPurpose) SearchInterpretation {
	out := emptyInterpretation(interpretationSkipped, filters)
	if purpose == searchForReference {
		return out
	}
	out.Status, out.FallbackReason = interpretationFallback, fallbackUnavailable
	if s.IntentAnalyzer == nil {
		return out
	}
	analysisCtx, cancel := context.WithTimeout(ctx, intentDeadline)
	defer cancel()
	analysis, err := s.IntentAnalyzer.AnalyzeIntent(analysisCtx, query, s.Budgets.Within(ctx, IntentBudget))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(analysisCtx.Err(), context.DeadlineExceeded) {
			out.FallbackReason = fallbackTimeout
		}
		return out
	}
	if analysis == nil {
		out.FallbackReason = fallbackInvalidResponse
		return out
	}
	out.Model, out.PromptVersion = analysis.Interpretation.Model, analysis.Interpretation.PromptVersion
	s.recordVersionedCallCost(ctx, credit.KindSearchIntent, out.Model, out.PromptVersion, analysis.Usage)
	if err := analysis.Interpretation.validate(query, extractedByModel); err != nil || !analysis.Valid ||
		out.Model == "" || out.PromptVersion == "" {
		out.FallbackReason = fallbackInvalidResponse
		return out
	}
	out = analysis.Interpretation
	out.Status, out.FallbackReason = interpretationAnalyzed, ""
	for key, value := range filters.values() {
		out.Filters[key] = value
	}
	return out
}

func (f searchFilters) values() map[string]string {
	values := make(map[string]string)
	if f.HasScript != nil {
		values[filterScript] = "no"
		if *f.HasScript {
			values[filterScript] = "yes"
		}
	}
	if f.SpecValidated != nil {
		values[filterValidation] = compatUnverified
		if *f.SpecValidated {
			values[filterValidation] = compatPassed
		}
	}
	for key, value := range map[string]*string{filterAgent: f.AgentRuntime, filterTier: f.CurationTier, filterCategory: f.Category} {
		if value != nil {
			values[key] = *value
		}
	}
	return values
}
