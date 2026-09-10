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

// fakeLedger stands in for credit.Service. It records rather than writes, so
// these tests assert what catalog HANDED the ledger — which is the half
// catalog owns. What the ledger then does with it is credit's own tests.
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

// CRED-005 clause 1, catalog's row: one embedding call, exactly one cost event
// of the kind that names it.
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
	// $0.00001 is 10 micro-dollars, and it is rounded up rather than to
	// nearest (ADR-068 decision 6) — a search that priced below one micro
	// still costs one, never zero.
	if e.UsdMicros != 10 || e.Estimated {
		t.Errorf("usd_micros = %d estimated = %v, want 10 / false", e.UsdMicros, e.Estimated)
	}
	if e.Model != "text-embedding-3-small" || e.PromptTokens != 7 {
		t.Errorf("model = %q prompt tokens = %d", e.Model, e.PromptTokens)
	}
}

// CRED-005 clause 3: public search has no session, so the row it writes has no
// workspace and no user — and it is still written. A ledger that only recorded
// attributable calls would under-report the platform's own spend by however
// much anonymous search costs, which is precisely the number nobody would
// notice was missing.
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
	// No usage came back, so the row says so rather than claiming the call was
	// free (money.go's UsageCost).
	if !e.Estimated || e.UsdMicros != 0 {
		t.Errorf("estimated = %v usd_micros = %d, want true / 0", e.Estimated, e.UsdMicros)
	}
}

// CRED-005 clause 2's counter-test: search for something unmistakable and go
// looking for it in every field of the row that was written. ADR-029's rule —
// what a search cost is the platform's fact, what somebody searched for is not
// — has to survive somebody adding a field to CostEvent later, so this
// marshals the whole struct rather than checking the fields it knows about.
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

// A ledger that is down must not take search down with it. The call has
// already been paid for by the time the write is attempted; failing the search
// would lose the result as well as the money.
func TestALedgerFailureDoesNotFailTheSearch(t *testing.T) {
	s := &Service{LLM: embedServer(t, nil), Credit: &fakeLedger{err: context.DeadlineExceeded}}
	if _, err := s.embedQuery(context.Background(), "still works"); err != nil {
		t.Errorf("embedQuery err = %v, want nil", err)
	}
}
