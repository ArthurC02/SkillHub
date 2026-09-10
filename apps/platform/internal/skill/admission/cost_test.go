package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

// fakeLedger records what ingest handed the ledger instead of writing it.
type fakeLedger struct {
	events []credit.CostEvent
	err    error
}

func (f *fakeLedger) RecordCost(_ context.Context, _ credit.DBTX, e credit.CostEvent) (string, bool, error) {
	f.events = append(f.events, e)
	return "id", false, f.err
}

func (f *fakeLedger) kinds() []string {
	out := make([]string, len(f.events))
	for i, e := range f.events {
		out[i] = e.Kind
	}
	return out
}

// CRED-005 clause 1, the index_enrich row. Enrichment is TWO paid calls — the
// enrichment itself and the embedding of what it produced — and each one gets
// its own cost event. Collapsing them into one row would make the per-call
// statistics behind the start gate (ADR-068 decision 8) describe a call that
// does not exist.
func TestEnrichmentRecordsOneCostEventPerPaidCall(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	ledger := &fakeLedger{}
	s := &Service{LLM: stub.start(t), Credit: ledger}
	ws := mustUUIDForTest(t, "11111111-1111-1111-1111-111111111111")

	if e := s.enrichPackage(context.Background(), testPackage(), ws); e.status != enrichmentEnriched {
		t.Fatalf("enrichment status = %q, want enriched", e.status)
	}

	if got := ledger.kinds(); len(got) != 2 || got[0] != credit.KindIndexEnrich || got[1] != credit.KindIndexEnrich {
		t.Fatalf("cost event kinds = %v, want two %q", got, credit.KindIndexEnrich)
	}
	if m := ledger.events[0].Model; m != "gpt-5.6-sol" {
		t.Errorf("enrichment call model = %q", m)
	}
	if m := ledger.events[1].Model; m != "text-embedding-3-small" {
		t.Errorf("embedding call model = %q", m)
	}
	for i, e := range ledger.events {
		if e.WorkspaceID != ws {
			t.Errorf("event %d workspace = %v, want the importing workspace", i, e.WorkspaceID)
		}
		if e.IdempotencyKey == "" {
			t.Errorf("event %d has no idempotency key", i)
		}
	}
	if ledger.events[0].IdempotencyKey == ledger.events[1].IdempotencyKey {
		// Two calls sharing a key means the second row is silently dropped by
		// the unique constraint, which is the failure this pair of rows exists
		// to avoid.
		t.Error("both calls used the same idempotency key")
	}
}

// The enrichment call still happened when the embedding after it fails, so its
// cost is still recorded. Enrichment degrades to a pending document rather
// than an error (enrich.go), and the ledger follows the call, not the outcome.
func TestAFailedEmbeddingStillLeavesTheEnrichmentCallRecorded(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusInternalServerError}
	ledger := &fakeLedger{}
	s := &Service{LLM: stub.start(t), Credit: ledger}

	if e := s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{}); e.status != enrichmentPending {
		t.Fatalf("enrichment status = %q, want pending", e.status)
	}
	if got := ledger.kinds(); len(got) != 1 || got[0] != credit.KindIndexEnrich {
		t.Fatalf("cost event kinds = %v, want one %q", got, credit.KindIndexEnrich)
	}
}

// CRED-005 clause 2's counter-test on this path. The enrichment request
// carries the whole of SKILL.md and the response carries model prose; neither
// may reach the spend ledger.
func TestEnrichmentCostEventsCarryNoPackageOrModelText(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	ledger := &fakeLedger{}
	s := &Service{LLM: stub.start(t), Credit: ledger}

	s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{})

	blob, err := json.Marshal(ledger.events)
	if err != nil {
		t.Fatalf("marshal cost events: %v", err)
	}
	for _, forbidden := range []string{testEnrichedSummary, testExampleZh, testExampleEn, testLimitation, testDescription} {
		if strings.Contains(string(blob), forbidden) {
			t.Errorf("%q reached the cost ledger: %s", forbidden, blob)
		}
	}
}

// A ledger that refuses must not turn a successful enrichment into a pending
// document: the enrichment is what the import path needs, and the spend row is
// what accounting needs.
func TestALedgerFailureDoesNotDegradeTheEnrichment(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	s := &Service{LLM: stub.start(t), Credit: &fakeLedger{err: context.DeadlineExceeded}}

	if e := s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{}); e.status != enrichmentEnriched {
		t.Errorf("enrichment status = %q, want enriched", e.status)
	}
}

// CRED-005 clause 1, the generate row. The cost is recorded by the function
// that makes the call, so a two-attempt generation leaves two rows: the retry
// is not free (ADR-047 決策 1) and a ledger that priced a generation instead of
// its calls would report the second attempt as costing nothing.
func TestEachGenerationCallRecordsItsOwnCostEvent(t *testing.T) {
	const body = `{"skill":{"name":"a","description":"b","body":"c"},` +
		`"model":"gen-model","prompt_version":"generate/v3",` +
		`"usage":{"prompt_tokens":1200,"completion_tokens":800,"cost_usd":0.0045,"cost_source":"gateway"}}`
	ledger := &fakeLedger{}
	svc := gatewayReturning(t, body)
	svc.Credit = ledger
	ws := mustUUIDForTest(t, "22222222-2222-2222-2222-222222222222")

	for range 2 {
		if _, err := svc.generateOnce(context.Background(), ws, "把掃描的單據整理成表格。", nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	if got := ledger.kinds(); len(got) != 2 || got[0] != credit.KindGenerate || got[1] != credit.KindGenerate {
		t.Fatalf("cost event kinds = %v, want two %q", got, credit.KindGenerate)
	}
	e := ledger.events[0]
	// $0.0045 is 4500 micro-dollars — ADR-068's own worked example for a
	// single generation.
	if e.UsdMicros != 4500 || e.Estimated {
		t.Errorf("usd_micros = %d estimated = %v, want 4500 / false", e.UsdMicros, e.Estimated)
	}
	if e.Model != "gen-model" || e.PromptVersion != "generate/v3" || e.WorkspaceID != ws {
		t.Errorf("provenance = %q/%q/%v", e.Model, e.PromptVersion, e.WorkspaceID)
	}
	if ledger.events[0].IdempotencyKey == ledger.events[1].IdempotencyKey {
		t.Error("both attempts used the same idempotency key")
	}
}

// The task description is the user's own words and never reaches the ledger.
func TestGenerationCostEventCarriesNoTaskDescription(t *testing.T) {
	const secret = "an-unannounced-internal-project-name"
	ledger := &fakeLedger{}
	svc := gatewayReturning(t, `{"skill":{"name":"a","description":"b","body":"c"},"model":"m","prompt_version":"v"}`)
	svc.Credit = ledger

	if _, err := svc.generateOnce(context.Background(), pgtype.UUID{},
		"請幫我做一個處理 "+secret+" 報表的工具。", nil, nil); err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(ledger.events)
	if err != nil {
		t.Fatalf("marshal cost events: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Errorf("the task description reached the cost ledger: %s", blob)
	}
}
