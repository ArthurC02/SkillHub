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
	usage    *ModelUsage
	asked    CriteriaRequest
}

func (f *fakeSuggester) SuggestCriteria(_ context.Context, req CriteriaRequest) (*CriteriaProposal, error) {
	f.asked = req
	return &CriteriaProposal{Texts: f.proposed, Usage: f.usage}, nil
}

func criteriaOverAWireServer(t *testing.T, asked *llmclient.SuggestCriteriaRequest, reply []string) CriteriaSuggester {
	t.Helper()
	return criteriaOverAWireServerWithUsage(t, asked, reply, nil)
}

func criteriaOverAWireServerWithUsage(t *testing.T, asked *llmclient.SuggestCriteriaRequest,
	reply []string, usage *llmclient.GatewayUsage) CriteriaSuggester {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(asked); err != nil {
			t.Errorf("decoding the request the adapter sent: %v", err)
		}
		out := llmclient.SuggestCriteriaResponse{Usage: usage}
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
			if len(got.Texts) != len(want) {
				t.Fatalf("proposed = %q, want %q", got.Texts, want)
			}
			for i := range want {
				if got.Texts[i] != want[i] {
					t.Errorf("proposed[%d] = %q, want %q", i, got.Texts[i], want[i])
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

type recordedSpend struct{ seen []*ModelUsage }

func (r *recordedSpend) record(_ context.Context, u *ModelUsage) error {
	r.seen = append(r.seen, u)
	return nil
}

func TestWhatTheGatewayChargedForAProposalIsRecorded(t *testing.T) {
	cost := 0.002
	for _, tc := range []struct {
		name         string
		usage        *llmclient.GatewayUsage
		wantReported *float64
	}{
		{"the gateway priced it", &llmclient.GatewayUsage{
			PromptTokens: 90, CompletionTokens: 12, CostUSD: &cost, CostSource: llmclient.CostSourceGateway,
		}, &cost},
		{"something else priced it", &llmclient.GatewayUsage{
			PromptTokens: 90, CompletionTokens: 12, CostUSD: &cost, CostSource: "estimated",
		}, nil},
		{"the gateway reported nothing", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := &recordedSpend{}
			var asked llmclient.SuggestCriteriaRequest
			s := &Service{
				LLM:         criteriaOverAWireServerWithUsage(t, &asked, []string{"the total is a number"}, tc.usage),
				RecordSpend: ledger.record,
			}

			proposal, err := s.LLM.SuggestCriteria(context.Background(), aCriteriaRequest())
			if err != nil {
				t.Fatalf("suggesting criteria: %v", err)
			}
			s.recordSuggestCost(context.Background(), proposal.Usage)

			if len(ledger.seen) != 1 {
				t.Fatalf("spend records = %d, want 1: a paid proposal must reach the ledger", len(ledger.seen))
			}
			got := ledger.seen[0]
			reported := got.ReportedCostUSD()
			if (reported == nil) != (tc.wantReported == nil) {
				t.Fatalf("reported cost = %v, want %v", reported, tc.wantReported)
			}
			if reported != nil && *reported != *tc.wantReported {
				t.Errorf("reported cost = %v, want %v", *reported, *tc.wantReported)
			}
			if tc.usage != nil && (got.PromptTokens != 90 || got.CompletionTokens != 12) {
				t.Errorf("tokens = %d/%d, want 90/12", got.PromptTokens, got.CompletionTokens)
			}
		})
	}
}
