package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAGeneralSummaryIsIncompleteOnlyWhenSomeStreamMissesAnEvent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		health []StreamHealth
		want   bool
	}{
		{"no streams at all", nil, false},
		{"every stream gapless", []StreamHealth{{MissingCount: 0}, {MissingCount: 0}}, false},
		{"one event missing on the last stream", []StreamHealth{{MissingCount: 0}, {MissingCount: 1}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyStreamMissing(tc.health); got != tc.want {
				t.Errorf("anyStreamMissing = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAProgressStepCarriesTheTransitionReasonOnlyWhenThereIsOne(t *testing.T) {
	blank, given := "", "run requested"
	for _, tc := range []struct {
		name       string
		transition RunTransition
		want       ProgressStep
	}{
		{"no reason recorded", RunTransition{ToStatus: "queued"}, ProgressStep{Status: "queued"}},
		{"an empty reason", RunTransition{ToStatus: "running", Reason: &blank}, ProgressStep{Status: "running"}},
		{"a reason", RunTransition{ToStatus: "queued", Reason: &given}, ProgressStep{Status: "queued", Reason: "run requested"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := progressStepOf(tc.transition); got != tc.want {
				t.Errorf("progressStepOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestASettledCostReadFailureKeepsTheSandboxsReading(t *testing.T) {
	sandbox := 0.5
	usage := &UsageSummary{CostUSD: &sandbox, CostSource: "gateway"}
	s := &Service{SettledCostUSD: func(context.Context, pgtype.UUID) (float64, bool, error) {
		return 0.6, true, errors.New("ledger unavailable")
	}}

	s.preferSettledCost(context.Background(), pgtype.UUID{}, usage)

	if usage.CostUSD == nil || *usage.CostUSD != 0.5 {
		t.Errorf("cost = %v, want the sandbox's 0.5 kept when the ledger cannot be read", usage.CostUSD)
	}
	if usage.CostSource != "gateway" {
		t.Errorf("cost_source = %q, want gateway", usage.CostSource)
	}
}
