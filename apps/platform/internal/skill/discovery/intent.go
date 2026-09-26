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

func emptyInterpretation(status string, filters searchFilters) SearchInterpretation {
	return SearchInterpretation{
		Status:   status,
		Intent:   map[string]*string{"input": nil, "output": nil, "tools": nil, "data": nil, "environment": nil},
		Keywords: []string{}, Filters: filters.values(),
	}
}

func (i SearchInterpretation) validate(query string, extracted bool) (searchFilters, error) {
	if len(i.Intent) != 5 || i.Keywords == nil || i.Filters == nil {
		return searchFilters{}, errors.New("intent, keywords and filters are required")
	}
	for _, field := range []string{"input", "output", "tools", "data", "environment"} {
		value, exists := i.Intent[field]
		if !exists {
			return searchFilters{}, errors.New("all five intent fields are required")
		}
		if value != nil && (strings.TrimSpace(*value) == "" || utf8.RuneCountInString(*value) > maxQueryRunes ||
			(extracted && !strings.Contains(query, *value))) {
			return searchFilters{}, errors.New("invalid intent field")
		}
	}
	if len(i.Keywords) > maxIntentKeywords {
		return searchFilters{}, errors.New("too many search keywords")
	}
	for _, keyword := range i.Keywords {
		if strings.TrimSpace(keyword) == "" || utf8.RuneCountInString(keyword) > maxIntentKeywordRunes {
			return searchFilters{}, errors.New("invalid search keyword")
		}
	}
	for key := range i.Filters {
		switch key {
		case "script", "validation", "agent", "tier", "category":
		default:
			return searchFilters{}, errors.New("unsupported search filter")
		}
	}
	if !extracted && utf8.RuneCountInString(i.retrievalQuery(query)) > maxQueryRunes {
		return searchFilters{}, errors.New("combined intent and keywords must not exceed 2000 characters")
	}
	return parseFilterValues(i.Filters)
}

func (i SearchInterpretation) retrievalQuery(original string) string {
	var terms []string
	if i.Status == "corrected" {
		for _, field := range []string{"input", "output", "tools", "data", "environment"} {
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

func (s *Service) interpret(ctx context.Context, query string, filters searchFilters, silent bool) SearchInterpretation {
	out := emptyInterpretation("skipped", filters)
	if silent {
		return out
	}
	out.Status, out.FallbackReason = "fallback", "unavailable"
	if s.IntentAnalyzer == nil {
		return out
	}
	analysisCtx, cancel := context.WithTimeout(ctx, intentDeadline)
	defer cancel()
	analysis, err := s.IntentAnalyzer.AnalyzeIntent(analysisCtx, query, s.Budgets.Within(ctx, IntentBudget))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(analysisCtx.Err(), context.DeadlineExceeded) {
			out.FallbackReason = "timeout"
		}
		return out
	}
	if analysis == nil {
		out.FallbackReason = "invalid_response"
		return out
	}
	out.Model, out.PromptVersion = analysis.Interpretation.Model, analysis.Interpretation.PromptVersion
	s.recordVersionedCallCost(ctx, credit.KindSearchIntent, out.Model, out.PromptVersion, analysis.Usage)
	if _, err := analysis.Interpretation.validate(query, true); err != nil || !analysis.Valid ||
		out.Model == "" || out.PromptVersion == "" {
		out.FallbackReason = "invalid_response"
		return out
	}
	out = analysis.Interpretation
	out.Status, out.FallbackReason = "analyzed", ""
	for key, value := range filters.values() {
		out.Filters[key] = value
	}
	return out
}

func (f searchFilters) values() map[string]string {
	values := make(map[string]string)
	if f.HasScript != nil {
		values["script"] = "no"
		if *f.HasScript {
			values["script"] = "yes"
		}
	}
	if f.SpecValidated != nil {
		values["validation"] = "unverified"
		if *f.SpecValidated {
			values["validation"] = "passed"
		}
	}
	for key, value := range map[string]*string{"agent": f.AgentRuntime, "tier": f.CurationTier, "category": f.Category} {
		if value != nil {
			values[key] = *value
		}
	}
	return values
}
