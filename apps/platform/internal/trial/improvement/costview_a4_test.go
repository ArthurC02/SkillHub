package eval

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// The read side's default cost source has to be a value the write side can
// produce. costSource() writes a source only for a gateway-reported figure and
// NULL otherwise, and judge.go states the rule outright: the internal contract
// has no `estimated` source, and an unrecognised label is unreported accounting
// rather than permission to relabel it as an estimate.
//
// The default here used to be "estimated", so every evaluation the gateway never
// priced was rendered as {"evaluation_usd": null, "source": "estimated"} — one
// field claiming the platform estimated something, next to that view's own note
// saying it has no figure at all. NFR-001: the screen has to pick one to believe.
func TestAnUnpricedEvaluationIsUnreportedAndNotAnEstimate(t *testing.T) {
	v := costViewOf(gen.Evaluation{})
	if v.Source == "estimated" {
		t.Fatal("an evaluation with no gateway figure was labelled an estimate; nothing estimated anything")
	}
	if v.Source != "unreported" {
		t.Errorf("source = %q, want %q — the value domain costSource() writes", v.Source, "unreported")
	}
	if v.EvaluationUSD != nil {
		t.Errorf("evaluation_usd = %v, want null", *v.EvaluationUSD)
	}
	if v.Note == "" {
		t.Error("an unpriced evaluation carries no note saying it is unreported and not $0")
	}

	// The two halves stay consistent: whatever costSource() did write is what
	// comes back out, so this default can never contradict a real label.
	gateway := "gateway"
	var cost pgtype.Numeric
	if err := cost.Scan("0.0123"); err != nil {
		t.Fatal(err)
	}
	priced := costViewOf(gen.Evaluation{CostSource: &gateway, CostUsd: cost})
	if priced.Source != "gateway" {
		t.Errorf("source = %q, want the stored label %q", priced.Source, "gateway")
	}
	if priced.EvaluationUSD == nil || *priced.EvaluationUSD != 0.0123 {
		t.Errorf("evaluation_usd = %v, want 0.0123", priced.EvaluationUSD)
	}
}
