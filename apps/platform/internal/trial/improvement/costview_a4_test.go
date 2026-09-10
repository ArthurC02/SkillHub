package eval

import (
	"math"
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
// priced was rendered as {"evaluation_credits": null, "source": "estimated"} — one
// field claiming the platform estimated something, next to that view's own note
// saying it has no figure at all. NFR-001: the screen has to pick one to believe.
func TestAnUnpricedEvaluationIsUnreportedAndNotAnEstimate(t *testing.T) {
	v := costViewOf(gen.Evaluation{}, testCredits)
	if v.Source == "estimated" {
		t.Fatal("an evaluation with no gateway figure was labelled an estimate; nothing estimated anything")
	}
	if v.Source != "unreported" {
		t.Errorf("source = %q, want %q — the value domain costSource() writes", v.Source, "unreported")
	}
	if v.EvaluationCredits != nil {
		t.Errorf("evaluation_credits = %v, want null", *v.EvaluationCredits)
	}
	if v.Note == "" {
		t.Error("an unpriced evaluation carries no note saying it is unreported and not 0 點")
	}

	// The two halves stay consistent: whatever costSource() did write is what
	// comes back out, so this default can never contradict a real label.
	gateway := "gateway"
	var cost pgtype.Numeric
	if err := cost.Scan("0.0123"); err != nil {
		t.Fatal(err)
	}
	priced := costViewOf(gen.Evaluation{CostSource: &gateway, CostUsd: cost}, testCredits)
	if priced.Source != "gateway" {
		t.Errorf("source = %q, want the stored label %q", priced.Source, "gateway")
	}
	// $0.0123 with the shipped 1.3x markup at US$0.001 per credit is 15.99
	// credits, and the conversion rounds up (ADR-068 decision 6): 16.
	if priced.EvaluationCredits == nil || *priced.EvaluationCredits != 16 {
		t.Errorf("evaluation_credits = %v, want 16", priced.EvaluationCredits)
	}
}

// testCredits is the shipped ADR-068 rate: US$0.001 per credit, 1.3x markup,
// rounding up. Written out rather than built from credit.ConfigFromEnv so that
// a change to the defaults shows up here as a failing number rather than as a
// test that silently agrees with whatever the code now does.
func testCredits(usd float64) (int64, bool) {
	micros := int64(math.Ceil(usd * 1_000_000))
	billed := (micros*13000 + 9999) / 10000
	return (billed + 999) / 1000, true
}
