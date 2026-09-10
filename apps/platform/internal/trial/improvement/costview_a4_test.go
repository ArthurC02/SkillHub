package eval

import (
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

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

	gateway := "gateway"
	var cost pgtype.Numeric
	if err := cost.Scan("0.0123"); err != nil {
		t.Fatal(err)
	}
	priced := costViewOf(gen.Evaluation{CostSource: &gateway, CostUsd: cost}, testCredits)
	if priced.Source != "gateway" {
		t.Errorf("source = %q, want the stored label %q", priced.Source, "gateway")
	}

	if priced.EvaluationCredits == nil || *priced.EvaluationCredits != 16 {
		t.Errorf("evaluation_credits = %v, want 16", priced.EvaluationCredits)
	}
}

func testCredits(usd float64) (int64, bool) {
	micros := int64(math.Ceil(usd * 1_000_000))
	billed := (micros*13000 + 9999) / 10000
	return (billed + 999) / 1000, true
}
