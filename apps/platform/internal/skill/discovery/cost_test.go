package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeLedger struct {
	events []credit.CostEvent
	err    error
}

func (f *fakeLedger) RecordCost(_ context.Context, _ credit.DBTX, e credit.CostEvent) (string, bool, error) {
	f.events = append(f.events, e)
	return "id", false, f.err
}

func embedServer(t *testing.T, usage *llmclient.GatewayUsage) *llmclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.EmbedResponse{
			Embeddings: [][]float32{make([]float32, 1536)},
			Model:      "text-embedding-3-small",
			Dimensions: 1536,
			Usage:      usage,
		})
	}))
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

func TestSearchEmbeddingRecordsExactlyOneCostEvent(t *testing.T) {
	cost := 0.00001
	ledger := &fakeLedger{}
	s := &Service{LLM: embedServer(t, &llmclient.GatewayUsage{
		PromptTokens: 7, CostUSD: &cost, CostSource: "gateway",
	}), Credit: ledger}

	if _, err := s.embedQuery(context.Background(), "how do I extract tables"); err != nil {
		t.Fatalf("embedQuery: %v", err)
	}

	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	e := ledger.events[0]
	if e.Kind != credit.KindSearchEmbedding {
		t.Errorf("kind = %q, want %q", e.Kind, credit.KindSearchEmbedding)
	}

	if e.UsdMicros != 10 || e.Estimated {
		t.Errorf("usd_micros = %d estimated = %v, want 10 / false", e.UsdMicros, e.Estimated)
	}
	if e.Model != "text-embedding-3-small" || e.PromptTokens != 7 {
		t.Errorf("model = %q prompt tokens = %d", e.Model, e.PromptTokens)
	}
}

func TestAnonymousSearchStillRecordsCostWithBothIdsNull(t *testing.T) {
	ledger := &fakeLedger{}
	s := &Service{LLM: embedServer(t, nil), Credit: ledger}

	if _, err := s.embedQuery(context.Background(), "anything"); err != nil {
		t.Fatalf("embedQuery: %v", err)
	}
	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	e := ledger.events[0]
	if e.WorkspaceID.Valid || e.UserID.Valid {
		t.Errorf("workspace/user = %v/%v, want both null", e.WorkspaceID.Valid, e.UserID.Valid)
	}

	if !e.Estimated || e.UsdMicros != 0 {
		t.Errorf("estimated = %v usd_micros = %d, want true / 0", e.Estimated, e.UsdMicros)
	}
}

func TestCostEventNeverCarriesTheQueryText(t *testing.T) {
	const secret = "my-employers-unannounced-product-codename"
	ledger := &fakeLedger{}
	s := &Service{LLM: embedServer(t, nil), Credit: ledger}

	if _, err := s.embedQuery(context.Background(), "how do I "+secret); err != nil {
		t.Fatalf("embedQuery: %v", err)
	}
	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	blob, err := json.Marshal(ledger.events[0])
	if err != nil {
		t.Fatalf("marshal cost event: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Errorf("the query text reached the cost event: %s", blob)
	}
}

func TestALedgerFailureDoesNotFailTheSearch(t *testing.T) {
	s := &Service{LLM: embedServer(t, nil), Credit: &fakeLedger{err: context.DeadlineExceeded}}
	if _, err := s.embedQuery(context.Background(), "still works"); err != nil {
		t.Errorf("embedQuery err = %v, want nil", err)
	}
}
