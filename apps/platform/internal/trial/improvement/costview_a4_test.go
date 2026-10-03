package eval

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func creditsStub(credits int64, ok bool, asked *[]float64) func(float64) (int64, bool) {
	return func(usd float64) (int64, bool) {
		*asked = append(*asked, usd)
		return credits, ok
	}
}

func TestAnUnpricedEvaluationIsUnreportedAndNotAnEstimate(t *testing.T) {
	var asked []float64
	v := costViewOf(EvaluationRecord{}, creditsStub(16, true, &asked))
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
	if len(asked) != 0 {
		t.Errorf("the price converter was asked about %v for an evaluation with no cost", asked)
	}

	gateway := "gateway"
	var cost pgtype.Numeric
	if err := cost.Scan("0.0123"); err != nil {
		t.Fatal(err)
	}
	priced := costViewOf(EvaluationRecord{CostSource: &gateway, CostUSD: cost}, creditsStub(16, true, &asked))
	if priced.Source != "gateway" {
		t.Errorf("source = %q, want the stored label %q", priced.Source, "gateway")
	}

	if priced.EvaluationCredits == nil || *priced.EvaluationCredits != 16 {
		t.Errorf("evaluation_credits = %v, want 16", priced.EvaluationCredits)
	}
	if len(asked) != 1 || asked[0] != 0.0123 {
		t.Errorf("the converter was asked about %v, want exactly [0.0123]", asked)
	}

	refused := costViewOf(EvaluationRecord{CostSource: &gateway, CostUSD: cost}, creditsStub(16, false, &asked))
	if refused.EvaluationCredits != nil {
		t.Errorf("evaluation_credits = %v, want null when the converter cannot price it", *refused.EvaluationCredits)
	}
}
