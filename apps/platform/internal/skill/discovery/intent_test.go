package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

type intentAnalyzerFunc func(context.Context, string, time.Duration) (*IntentAnalysis, error)

func (f intentAnalyzerFunc) AnalyzeIntent(ctx context.Context, query string, within time.Duration) (*IntentAnalysis, error) {
	return f(ctx, query, within)
}

func validInterpretation() SearchInterpretation {
	i := emptyInterpretation("analyzed", searchFilters{})
	i.Model, i.PromptVersion = "small-model", "search-intent/v1"
	i.Keywords = []string{"CSV"}
	input := "CSV"
	i.Intent["input"] = &input
	return i
}

func TestIntentValidationRejectsIncompleteInventedAndOversizedProposals(t *testing.T) {
	cases := []struct {
		name   string
		change func(*SearchInterpretation)
	}{
		{"missing field", func(i *SearchInterpretation) { delete(i.Intent, "tools") }},
		{"unknown field", func(i *SearchInterpretation) { delete(i.Intent, "tools"); i.Intent["unknown"] = nil }},
		{"invented tool", func(i *SearchInterpretation) { s := "Python"; i.Intent["tools"] = &s }},
		{"empty field", func(i *SearchInterpretation) { s := ""; i.Intent["tools"] = &s }},
		{"blank field", func(i *SearchInterpretation) { s := " "; i.Intent["tools"] = &s }},
		{"long field", func(i *SearchInterpretation) { s := strings.Repeat("文", 2001); i.Intent["input"] = &s }},
		{"missing keywords", func(i *SearchInterpretation) { i.Keywords = nil }},
		{"too many keywords", func(i *SearchInterpretation) { i.Keywords = strings.Fields(strings.Repeat("文 ", 9)) }},
		{"long keyword", func(i *SearchInterpretation) { i.Keywords = []string{strings.Repeat("文", 129)} }},
		{"blank keyword", func(i *SearchInterpretation) { i.Keywords = []string{" "} }},
		{"missing filters", func(i *SearchInterpretation) { i.Filters = nil }},
		{"unknown filter", func(i *SearchInterpretation) { i.Filters["mcp"] = "yes" }},
		{"invalid filter", func(i *SearchInterpretation) { i.Filters["tier"] = "external" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := validInterpretation()
			tc.change(&i)
			if _, err := i.validate("CSV", true); err == nil {
				t.Fatal("invalid proposal was accepted")
			}
		})
	}
}

func TestIntentLimitsAcceptTheBoundaryAndUserCorrectionsNeedNotQuoteOriginal(t *testing.T) {
	i := validInterpretation()
	text := strings.Repeat("文", 2000)
	i.Intent["input"] = &text
	i.Keywords = make([]string, 8)
	for n := range i.Keywords {
		i.Keywords[n] = strings.Repeat("文", 128)
	}
	if _, err := i.validate(text, true); err != nil {
		t.Fatal(err)
	}
	if _, err := i.validate("different original query", false); err != nil {
		t.Fatal(err)
	}
	i.Keywords = []string{}
	if got := i.retrievalQuery("original"); got != "original" {
		t.Fatalf("retrieval = %q", got)
	}
	i.Keywords = []string{"CSV", "報告"}
	if got := i.retrievalQuery("original"); got != "CSV 報告" {
		t.Fatalf("retrieval = %q", got)
	}
}

func TestCorrectedIntentRetrievesEveryFieldAndDeduplicatesKeywords(t *testing.T) {
	i := emptyInterpretation("corrected", searchFilters{})
	for field, value := range map[string]string{"input": "invoice", "output": "CSV", "tools": "Python", "data": "ledger", "environment": "offline"} {
		i.Intent[field] = &value
	}
	i.Keywords = []string{"invoice", "report", "report"}
	if got := i.retrievalQuery("original"); got != "invoice CSV Python ledger offline report" {
		t.Fatalf("retrieval = %q", got)
	}
	i.Keywords = []string{}
	if got := i.retrievalQuery("original"); got != "invoice CSV Python ledger offline" {
		t.Fatalf("field-only retrieval = %q", got)
	}
	i = emptyInterpretation("corrected", searchFilters{})
	if got := i.retrievalQuery("original"); got != "original" {
		t.Fatalf("overturned retrieval = %q", got)
	}
}

func TestCorrectedIntentCombinedTextLimit(t *testing.T) {
	for _, size := range []int{2000, 2001} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			i := emptyInterpretation("corrected", searchFilters{})
			value := strings.Repeat("文", size-2)
			i.Intent["input"] = &value
			i.Keywords = []string{"字"}
			_, err := i.validate("original", false)
			if (err != nil) != (size > 2000) {
				t.Fatalf("combined size=%d err=%v", size, err)
			}
		})
	}
}

func TestInterpretationUsesOneBoundedCallAndExplicitFiltersWin(t *testing.T) {
	calls := 0
	ledger := &fakeLedger{}
	s := &Service{Credit: ledger, IntentAnalyzer: intentAnalyzerFunc(func(ctx context.Context, query string, within time.Duration) (*IntentAnalysis, error) {
		calls++
		if query != "CSV" || within != 8*time.Second {
			t.Fatalf("query=%q within=%v", query, within)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		i := validInterpretation()
		i.Filters = map[string]string{"category": "data", "script": "yes"}
		return &IntentAnalysis{Valid: true, Interpretation: i}, nil
	})}
	category := "writing"
	got := s.interpret(context.Background(), "CSV", searchFilters{Category: &category}, false)
	if got.Status != "analyzed" || got.Filters["category"] != "writing" || got.Filters["script"] != "yes" || calls != 1 {
		t.Fatalf("interpretation=%+v calls=%d", got, calls)
	}
	if len(ledger.events) != 1 || ledger.events[0].Kind != credit.KindSearchIntent || ledger.events[0].PromptVersion != "search-intent/v1" {
		t.Fatalf("cost events=%+v", ledger.events)
	}
}

func TestInterpretationFailuresPreserveOriginalRetrievalAndUserFilters(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *IntentAnalysis
		err    error
		reason string
	}{
		{"unavailable", nil, errors.New("unavailable"), "unavailable"},
		{"timeout", nil, context.DeadlineExceeded, "timeout"},
		{"missing response", nil, nil, "invalid_response"},
		{"invalid result", &IntentAnalysis{}, nil, "invalid_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{IntentAnalyzer: intentAnalyzerFunc(func(context.Context, string, time.Duration) (*IntentAnalysis, error) { return tc.result, tc.err })}
			category := "documents"
			got := s.interpret(context.Background(), "CSV", searchFilters{Category: &category}, false)
			if got.Status != "fallback" || got.FallbackReason != tc.reason || got.retrievalQuery("CSV") != "CSV" || !reflect.DeepEqual(got.Filters, map[string]string{"category": "documents"}) {
				t.Fatalf("fallback=%+v", got)
			}
		})
	}
}

func TestReferencePickerDoesNotAnalyzeIntent(t *testing.T) {
	s := &Service{IntentAnalyzer: intentAnalyzerFunc(func(context.Context, string, time.Duration) (*IntentAnalysis, error) {
		t.Fatal("reference picker called analyzer")
		return nil, nil
	})}
	got := s.interpret(context.Background(), "CSV", searchFilters{}, true)
	if got.Status != "skipped" || got.retrievalQuery("CSV") != "CSV" {
		t.Fatalf("interpretation=%+v", got)
	}
}

func TestCorrectedSearchRejectsMalformedRequestsBeforeRetrieval(t *testing.T) {
	valid := `"query":"CSV","intent":{"input":null,"output":null,"tools":null,"data":null,"environment":null},"keywords":[],"filters":{}`
	for _, body := range []string{"{}", "null", "{" + valid + `,"limit":0}`, "{" + valid + `,"limit":101}`, "{" + valid + `,"workspace_id":"private"}`, "{" + valid + "} {}"} {
		t.Run(body, func(t *testing.T) {
			w := httptest.NewRecorder()
			(&Handler{Svc: &Service{}}).CorrectedSearch(w, httptest.NewRequest(http.MethodPost, "/api/skills/search", strings.NewReader(body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
