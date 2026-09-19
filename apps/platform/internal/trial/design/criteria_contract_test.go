package testlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeSuggester struct {
	proposed []string
	asked    CriteriaRequest
}

func (f *fakeSuggester) SuggestCriteria(_ context.Context, req CriteriaRequest) ([]string, error) {
	f.asked = req
	return f.proposed, nil
}

func criteriaOverAWireServer(t *testing.T, asked *llmclient.SuggestCriteriaRequest, reply []string) CriteriaSuggester {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(asked); err != nil {
			t.Errorf("decoding the request the adapter sent: %v", err)
		}
		out := llmclient.SuggestCriteriaResponse{}
		for _, text := range reply {
			out.Criteria = append(out.Criteria, llmclient.SuggestedCriterion{Text: text})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func aCriteriaRequest() CriteriaRequest {
	return CriteriaRequest{
		SkillName:    "invoice reader",
		SkillSummary: "reads invoices",
		UserPrompt:   "extract the totals",
		Datasets: []DatasetOutline{{
			FileName:    "rows.csv",
			ContentType: "text/csv",
			Fields:      []DatasetField{{Name: "total", InferredType: "number"}},
		}},
	}
}

func TestEverySuggesterAnswersInTheDomainsOwnWords(t *testing.T) {
	want := []string{"the total is a number", "the currency is named"}

	fake := &fakeSuggester{proposed: want}
	var overTheWire llmclient.SuggestCriteriaRequest
	real := criteriaOverAWireServer(t, &overTheWire, want)

	for name, suggester := range map[string]CriteriaSuggester{"fake": fake, "http adapter": real} {
		t.Run(name, func(t *testing.T) {
			got, err := suggester.SuggestCriteria(context.Background(), aCriteriaRequest())
			if err != nil {
				t.Fatalf("suggesting criteria: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("proposed = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("proposed[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestTheAdapterCarriesTheWholeRequestOntoTheWire(t *testing.T) {
	var sent llmclient.SuggestCriteriaRequest
	suggester := criteriaOverAWireServer(t, &sent, []string{"a criterion"})

	if _, err := suggester.SuggestCriteria(context.Background(), aCriteriaRequest()); err != nil {
		t.Fatalf("suggesting criteria: %v", err)
	}

	want := aCriteriaRequest()
	if sent.SkillName != want.SkillName || sent.SkillSummary != want.SkillSummary || sent.UserPrompt != want.UserPrompt {
		t.Errorf("request on the wire = %+v, want the skill, its summary and the prompt", sent)
	}
	if len(sent.Datasets) != 1 || sent.Datasets[0].FileName != "rows.csv" ||
		sent.Datasets[0].ContentType != "text/csv" {
		t.Fatalf("datasets on the wire = %+v, want the one outline the domain described", sent.Datasets)
	}
	if len(sent.Datasets[0].Fields) != 1 || sent.Datasets[0].Fields[0].Name != "total" ||
		sent.Datasets[0].Fields[0].InferredType != "number" {
		t.Errorf("fields on the wire = %+v, want the field the domain inferred", sent.Datasets[0].Fields)
	}
}

func TestAnAbsentModelDoesNotReachTheTestLabLookingPresent(t *testing.T) {
	var unconfigured *llmclient.Client

	if model := ModelOrNone(unconfigured); model != nil {
		t.Error("an unconfigured client arrived as a non-nil CriteriaSuggester; the `LLM == nil` guard on " +
			"suggesting criteria now passes and the first call panics")
	}
	if model := ModelOrNone(&llmclient.Client{}); model == nil {
		t.Error("a configured client did not reach the test lab; criteria would never be suggested")
	}
}
