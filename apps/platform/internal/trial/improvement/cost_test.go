package eval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// fakeLedger records what eval handed the ledger instead of writing it.
type fakeLedger struct {
	events []credit.CostEvent
	err    error
}

func (f *fakeLedger) RecordCost(_ context.Context, _ credit.DBTX, e credit.CostEvent) (string, bool, error) {
	f.events = append(f.events, e)
	return "id", false, f.err
}

// CRED-005 clause 1, the review row. The judge call is settled inside the
// transaction that commits the verdict, so this drives the real complete()
// against a real database rather than calling the recorder directly — the
// thing worth proving is that the spend row and the verdict cannot come apart.
func TestJudgingRecordsExactlyOneReviewCostEvent(t *testing.T) {
	ledger := &fakeLedger{}
	s := &Service{Pool: requireEvalDB(t), Credit: ledger}
	m := seedRun(t, s.Pool)

	cost := 0.0031
	v := aVerdict("the criteria were met", OverallMet)
	v.usage = &llmclient.GatewayUsage{
		PromptTokens: 4000, CompletionTokens: 600, CostUSD: &cost, CostSource: "gateway",
	}
	ev := beginAndComplete(t, s, m, v)

	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	e := ledger.events[0]
	if e.Kind != credit.KindReview {
		t.Errorf("kind = %q, want %q", e.Kind, credit.KindReview)
	}
	if e.UsdMicros != 3100 || e.Estimated {
		t.Errorf("usd_micros = %d estimated = %v, want 3100 / false", e.UsdMicros, e.Estimated)
	}
	if e.WorkspaceID != m.run.WorkspaceID {
		t.Errorf("workspace = %v, want the run's", e.WorkspaceID)
	}
	// The ref is the Run, not the evaluation: ADR-068 decision 3's ref_type is
	// a closed set of three and an evaluation is not one of them.
	if e.RefType != credit.RefRun || e.RefID != m.run.ID {
		t.Errorf("ref = %q/%v, want run/%v", e.RefType, e.RefID, m.run.ID)
	}
	// The evaluation id is the idempotency key, so a redelivered judge job
	// settles the same spend once.
	if !strings.Contains(e.IdempotencyKey, pgconv.UUIDString(ev.ID)) {
		t.Errorf("idempotency key %q does not name the evaluation", e.IdempotencyKey)
	}
}

// CRED-005 clause 2's counter-test on this path. A verdict summary is model
// prose about a user's run; nothing about it belongs in a spend ledger.
func TestReviewCostEventCarriesNoVerdictText(t *testing.T) {
	const secret = "the-agent-leaked-an-internal-hostname"
	ledger := &fakeLedger{}
	s := &Service{Pool: requireEvalDB(t), Credit: ledger}
	m := seedRun(t, s.Pool)

	beginAndComplete(t, s, m, aVerdict(secret, OverallMet))

	blob, err := json.Marshal(ledger.events)
	if err != nil {
		t.Fatalf("marshal cost events: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Errorf("the verdict summary reached the cost ledger: %s", blob)
	}
}

// CRED-005 clause 1, the suggestion row. This leg deliberately runs after the
// verdict has committed and on the pool rather than in a transaction, so its
// cost row is written the same way — see cost.go.
func TestSuggestingRecordsExactlyOneSuggestionCostEvent(t *testing.T) {
	cost := 0.0022
	ledger := &fakeLedger{}
	// A real evaluation row, because the usage row this leg writes alongside
	// the spend row has a foreign key to one.
	s := &Service{
		Pool:   requireEvalDB(t),
		Credit: ledger,
		Suggester: stubSuggester{resp: llmclient.SuggestImprovementsResponse{
			Model: "suggest-model", PromptVersion: "suggest/v2",
			Usage: &llmclient.GatewayUsage{
				PromptTokens: 3000, CompletionTokens: 900, CostUSD: &cost, CostSource: "gateway",
			},
		}},
	}
	m := seedRun(t, s.Pool)
	ev := beginAndComplete(t, s, m, aVerdict("not met", OverallNotMet))
	ledger.events = nil // drop the review row; this test is about the leg after it

	s.suggest(context.Background(), m, ev, costTestVerdict())

	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	e := ledger.events[0]
	if e.Kind != credit.KindSuggestion {
		t.Errorf("kind = %q, want %q", e.Kind, credit.KindSuggestion)
	}
	if e.UsdMicros != 2200 || e.Estimated {
		t.Errorf("usd_micros = %d estimated = %v, want 2200 / false", e.UsdMicros, e.Estimated)
	}
	if e.Model != "suggest-model" || e.PromptVersion != "suggest/v2" {
		t.Errorf("provenance = %q/%q", e.Model, e.PromptVersion)
	}
}

// A cost the gateway did not price is recorded as unpriced, not as free. Zero
// written where nothing was reported would enter the p95 the start gate reads
// as an observation that a judgement cost nothing.
func TestAnUnpricedCallIsRecordedAsEstimatedRatherThanFree(t *testing.T) {
	ledger := &fakeLedger{}
	s := &Service{
		Pool:   requireEvalDB(t),
		Credit: ledger,
		Suggester: stubSuggester{resp: llmclient.SuggestImprovementsResponse{
			Model: "m", PromptVersion: "v",
			Usage: &llmclient.GatewayUsage{PromptTokens: 10, CostSource: "estimated"},
		}},
	}
	m := seedRun(t, s.Pool)
	ev := beginAndComplete(t, s, m, aVerdict("not met", OverallNotMet))
	ledger.events = nil

	s.suggest(context.Background(), m, ev, costTestVerdict())

	if len(ledger.events) != 1 {
		t.Fatalf("cost events = %d, want exactly 1", len(ledger.events))
	}
	if e := ledger.events[0]; !e.Estimated || e.UsdMicros != 0 || e.PromptTokens != 10 {
		t.Errorf("estimated = %v usd_micros = %d prompt tokens = %d, want true / 0 / 10",
			e.Estimated, e.UsdMicros, e.PromptTokens)
	}
}

// costTestVerdict is the minimum a suggest leg needs to run: a not-met verdict
// with one finding, which is what makes the leg call the gateway at all.
func costTestVerdict() verdict {
	return verdict{overall: OverallNotMet, findings: []Finding{{
		Category: CategoryEffect, Severity: SeverityWarning, Message: "needs work",
		Evidence: []EvidenceRef{{Kind: KindAgentOutput, Excerpt: "wrote report.md", Available: true}},
	}}}
}
