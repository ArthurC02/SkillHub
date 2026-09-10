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

type fakeLedger struct {
	events []credit.CostEvent
	err    error
}

func (f *fakeLedger) RecordCost(_ context.Context, _ credit.DBTX, e credit.CostEvent) (string, bool, error) {
	f.events = append(f.events, e)
	return "id", false, f.err
}

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

	if e.RefType != credit.RefRun || e.RefID != m.run.ID {
		t.Errorf("ref = %q/%v, want run/%v", e.RefType, e.RefID, m.run.ID)
	}

	if !strings.Contains(e.IdempotencyKey, pgconv.UUIDString(ev.ID)) {
		t.Errorf("idempotency key %q does not name the evaluation", e.IdempotencyKey)
	}
}

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

func TestSuggestingRecordsExactlyOneSuggestionCostEvent(t *testing.T) {
	cost := 0.0022
	ledger := &fakeLedger{}

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
	ledger.events = nil

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

func costTestVerdict() verdict {
	return verdict{overall: OverallNotMet, findings: []Finding{{
		Category: CategoryEffect, Severity: SeverityWarning, Message: "needs work",
		Evidence: []EvidenceRef{{Kind: KindAgentOutput, Excerpt: "wrote report.md", Available: true}},
	}}}
}
