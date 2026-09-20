package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeModel struct {
	embeddings *Embeddings
	reasons    *MatchReasons

	askedTexts  []string
	askedWithin time.Duration
	askedQuery  string
	askedAbout  []SkillCandidate
}

func (f *fakeModel) Embed(_ context.Context, texts []string, within time.Duration) (*Embeddings, error) {
	f.askedTexts, f.askedWithin = texts, within
	return f.embeddings, nil
}

func (f *fakeModel) MatchReasons(_ context.Context, query string, candidates []SkillCandidate,
	_ time.Duration,
) (*MatchReasons, error) {
	f.askedQuery, f.askedAbout = query, candidates
	return f.reasons, nil
}

func modelOverAWireServer(t *testing.T, route string, reply any, captured *[]byte) Model {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != route {
			t.Errorf("adapter called %q, want %q", r.URL.Path, route)
		}
		if captured != nil {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			*captured = body
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func TestEveryModelReturnsVectorsInTheDomainsOwnShape(t *testing.T) {
	vectors := [][]float32{{0.5, 0.25}}
	fake := &fakeModel{embeddings: &Embeddings{Vectors: vectors, Model: "embed-1"}}
	overTheWire := modelOverAWireServer(t, "/embed",
		llmclient.EmbedResponse{Embeddings: vectors, Model: "embed-1", Dimensions: 2}, nil)

	for name, model := range map[string]Model{"fake": fake, "http adapter": overTheWire} {
		t.Run(name, func(t *testing.T) {
			got, err := model.Embed(context.Background(), []string{"invoices"}, 10*time.Second)
			if err != nil {
				t.Fatalf("embedding: %v", err)
			}
			if got.Model != "embed-1" || len(got.Vectors) != 1 || len(got.Vectors[0]) != 2 ||
				got.Vectors[0][0] != 0.5 {
				t.Fatalf("embeddings = %+v, want the one vector the model returned", got)
			}
		})
	}
}

func TestOnlyAFigureTheGatewayStatedCountsAsReported(t *testing.T) {
	cost := 0.004
	for _, tc := range []struct {
		name         string
		usage        *llmclient.GatewayUsage
		wantReported bool
		wantCharged  *float64
	}{
		{"the gateway priced it", &llmclient.GatewayUsage{CostUSD: &cost, CostSource: llmclient.CostSourceGateway}, true, &cost},
		{"something else priced it", &llmclient.GatewayUsage{CostUSD: &cost, CostSource: "estimated"}, false, nil},
		{"the gateway named but gave no figure", &llmclient.GatewayUsage{CostSource: llmclient.CostSourceGateway}, false, nil},
		{"no usage at all", nil, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := modelOverAWireServer(t, "/embed",
				llmclient.EmbedResponse{Embeddings: [][]float32{{1}}, Model: "embed-1", Usage: tc.usage}, nil)

			got, err := model.Embed(context.Background(), []string{"invoices"}, time.Second)
			if err != nil {
				t.Fatalf("embedding: %v", err)
			}
			if tc.usage == nil {
				if got.Usage != nil {
					t.Fatalf("usage = %+v, want none when the gateway reported none", got.Usage)
				}
				return
			}
			if got.Usage.CostReported != tc.wantReported {
				t.Errorf("cost reported = %v, want %v", got.Usage.CostReported, tc.wantReported)
			}
			charged := got.Usage.reportedCostUSD()
			if (charged == nil) != (tc.wantCharged == nil) || charged != nil && *charged != *tc.wantCharged {
				t.Errorf("chargeable cost = %v, want %v: only a figure the gateway stated may be billed",
					charged, tc.wantCharged)
			}
		})
	}
}

func TestTheAdapterCarriesEveryCandidateOntoTheWire(t *testing.T) {
	var sent []byte
	model := modelOverAWireServer(t, "/match-reasons",
		llmclient.MatchReasonsResponse{
			Reasons: []llmclient.MatchReason{{SkillID: "s1", Reason: "it reads invoices"}},
			Model:   "reason-1",
		}, &sent)

	got, err := model.MatchReasons(context.Background(), "invoice totals", []SkillCandidate{
		{SkillID: "s1", Name: "invoice reader", Summary: "reads invoices"},
	}, 7*time.Second)
	if err != nil {
		t.Fatalf("asking for match reasons: %v", err)
	}
	if len(got.Reasons) != 1 || got.Reasons[0].SkillID != "s1" || got.Reasons[0].Reason != "it reads invoices" {
		t.Fatalf("reasons = %+v, want the one reason the model gave", got.Reasons)
	}

	var wire llmclient.MatchReasonsRequest
	if err := json.Unmarshal(sent, &wire); err != nil {
		t.Fatalf("decoding what the adapter sent: %v (%s)", err, sent)
	}
	if wire.Query != "invoice totals" || len(wire.Candidates) != 1 ||
		wire.Candidates[0].SkillID != "s1" || wire.Candidates[0].Name != "invoice reader" ||
		wire.Candidates[0].Summary != "reads invoices" {
		t.Errorf("request on the wire = %+v, want the query and the whole candidate", wire)
	}
}

func TestAnAbsentModelDoesNotReachTheCatalogLookingPresent(t *testing.T) {
	var unconfigured *llmclient.Client

	if model := ModelOrNone(unconfigured); model != nil {
		t.Error("an unconfigured client arrived as a non-nil Model; the `LLM != nil` guard on search " +
			"now passes and the first call panics")
	}
	if model := ModelOrNone(&llmclient.Client{}); model == nil {
		t.Error("a configured client did not reach the catalog; search would never rank by meaning")
	}
}
