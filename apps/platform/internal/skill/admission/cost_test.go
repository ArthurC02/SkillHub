package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

type fakeLedger struct {
	events  []credit.CostEvent
	err     error
	ctxErrs []error
}

func (f *fakeLedger) RecordCost(ctx context.Context, _ credit.DBTX, e credit.CostEvent) (string, bool, error) {
	f.events = append(f.events, e)
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	return "id", false, f.err
}

func (f *fakeLedger) kinds() []credit.CostKind {
	out := make([]credit.CostKind, len(f.events))
	for i, e := range f.events {
		out[i] = e.Kind
	}
	return out
}

func TestAPaidCallIsRecordedEvenAfterTheCallerGaveUp(t *testing.T) {
	ledger := &fakeLedger{}
	s := &Service{Credit: ledger}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.recordCost(ctx, credit.KindGenerate, pgtype.UUID{}, modelCall{model: "gpt-6-luna"})

	if len(ledger.ctxErrs) != 1 {
		t.Fatalf("cost events = %d, want 1", len(ledger.ctxErrs))
	}
	if err := ledger.ctxErrs[0]; err != nil {
		t.Errorf("the ledger was handed a context already done (%v); the paid call would go unrecorded", err)
	}
}

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
	if m := ledger.events[0].Model; m != "gpt-6-luna" {
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

		t.Error("both calls used the same idempotency key")
	}
}

func TestAFailedEmbeddingStillLeavesTheEnrichmentCallRecorded(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusInternalServerError}
	ledger := &fakeLedger{}
	s := &Service{LLM: stub.start(t), Credit: ledger}

	if e := s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{}); e.embedding != nil {
		t.Fatal("an embedding was stored although the embed call failed")
	}
	if got := ledger.kinds(); len(got) != 1 || got[0] != credit.KindIndexEnrich {
		t.Fatalf("cost event kinds = %v, want one %q", got, credit.KindIndexEnrich)
	}
}

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

func TestALedgerFailureDoesNotDegradeTheEnrichment(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	s := &Service{LLM: stub.start(t), Credit: &fakeLedger{err: context.DeadlineExceeded}}

	if e := s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{}); e.status != enrichmentEnriched {
		t.Errorf("enrichment status = %q, want enriched", e.status)
	}
}

func pricedDraft() *GeneratedDraft {
	cost := 0.0045
	return &GeneratedDraft{
		Model: "gen-model", PromptVersion: "generate/v3",
		Usage: &ModelUsage{PromptTokens: 1200, CompletionTokens: 800, CostUSD: &cost, CostReported: true},
	}
}

func TestTheDeliveredGenerationIsChargedOnceUnderItsRequest(t *testing.T) {
	ws := mustUUIDForTest(t, "22222222-2222-2222-2222-222222222222")
	job := generation{ws: identity.Workspace{ID: ws}, requestID: "request-7"}

	c := deliveredCharge(job, pricedDraft())

	if c.IdempotencyKey != "generate:request-7" {
		t.Errorf("idempotency key = %q, want the request's own", c.IdempotencyKey)
	}
	if c.WorkspaceID != ws || c.Model != "gen-model" || c.PromptVersion != "generate/v3" {
		t.Errorf("charge = %+v, want the workspace and the gateway's model", c)
	}
	if c.Usage.ReportedCostUSD() == nil || *c.Usage.ReportedCostUSD() != 0.0045 {
		t.Errorf("charge usage = %+v, want the gateway's reported cost", c.Usage)
	}
}

func TestADeliveredGenerationIsChargedEvenAfterTheCallerGaveUp(t *testing.T) {
	var chargeCtxErr error
	charged := false
	billing := GenerateBilling{Charge: func(ctx context.Context, _ GenerationCharge) error {
		charged, chargeCtxErr = true, ctx.Err()
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reported := billing.charged(ctx, GenerationCharge{IdempotencyKey: "generate:request-1"})

	if !charged || !reported || chargeCtxErr != nil {
		t.Errorf("charged=%v ctx err=%v; the delivered generation would go unbilled", charged, chargeCtxErr)
	}
}

func TestGenerationChargeCarriesNoTaskDescription(t *testing.T) {
	const secret = "an-unannounced-internal-project-name"
	job := generation{task: "請幫我做一個處理 " + secret + " 報表的工具。", requestID: "request-1"}

	blob, err := json.Marshal(deliveredCharge(job, pricedDraft()))
	if err != nil {
		t.Fatalf("marshal charge: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Errorf("the task description reached the charge: %s", blob)
	}
}

func haltedBilling() GenerateBilling {
	return GenerateBilling{ModelCallsHalted: func(context.Context) bool { return true }}
}

func TestAnImportUnderAP1HaltLeavesEnrichmentPendingWithoutCallingTheModel(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	ledger := &fakeLedger{}
	s := &Service{LLM: stub.start(t), Credit: ledger, Billing: haltedBilling()}

	if e := s.enrichPackage(context.Background(), testPackage(), pgtype.UUID{}); e.status != enrichmentPending {
		t.Errorf("enrichment status = %q, want pending so the backfill picks it up after the halt", e.status)
	}
	if len(ledger.events) != 0 {
		t.Errorf("cost events = %v, want none: the model is not called under a P1 halt", ledger.kinds())
	}
}

func TestTheEnrichmentBackfillWaitsOutAP1Halt(t *testing.T) {
	listed := false
	s := &Service{
		LLM: &fakeModel{}, Billing: haltedBilling(),
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error { return nil },
		Worklist: EnrichmentWorklist{Pending: func(context.Context, int32) ([]PendingEnrichment, error) {
			listed = true
			return nil, nil
		}},
	}
	done, failed, err := s.ReindexPending(context.Background(), 10)
	if err != nil || done != 0 || failed != 0 {
		t.Errorf("ReindexPending = %d, %d, %v; want 0, 0, nil", done, failed, err)
	}
	if listed {
		t.Error("the backfill listed pending skills under a P1 halt; it would call the model for each")
	}
}
