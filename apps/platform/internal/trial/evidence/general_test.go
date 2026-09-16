package trace

import (
	"math"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type factRow = gen.ListTraceGeneralFactsRow

func repeated(n int, row factRow) []factRow {
	rows := make([]factRow, n)
	for i := range rows {
		rows[i] = row
		rows[i].Seq = int64(i + 1)
	}
	return rows
}

func TestTheGeneralFoldShowsTheFirstHundredSkillsAndErrorsAndCountsThemAll(t *testing.T) {
	for _, n := range []int{generalSkillsShown, generalSkillsShown + 1} {
		rows := append(
			repeated(n, factRow{EventType: TypeSkillActivation, SkillName: "excel-insert", Decision: "activated"}),
			repeated(n, factRow{EventType: TypeError, Code: "tool_failed"})...)
		rows = append(rows, factRow{EventType: TypeResourceRead}, factRow{EventType: TypeScriptLog})

		got := foldGeneral(rows).summary

		if got.SkillsTotal != n || len(got.Skills) != generalSkillsShown || got.Skills[0].Name != "excel-insert" {
			t.Errorf("%d activations: total %d, shown %d (%+v)", n, got.SkillsTotal, len(got.Skills), got.Skills[0])
		}
		if got.ErrorsTotal != n || len(got.Errors) != generalErrorsShown || got.Errors[0].Code != "tool_failed" {
			t.Errorf("%d errors: total %d, shown %d", n, got.ErrorsTotal, len(got.Errors))
		}
		if got.ResourceRead != 1 {
			t.Errorf("resources read = %d, want 1", got.ResourceRead)
		}
	}
}

func TestToolCallsCountOnlySucceededAsSuccessAndIgnoreDurationsThatAreNotWholeMilliseconds(t *testing.T) {
	rows := []factRow{
		{EventType: TypeToolCall, ToolName: "bash", Outcome: "succeeded", DurationMs: "300"},
		{EventType: TypeToolCall, ToolName: "read", Outcome: "failed", DurationMs: "500"},
		{EventType: TypeToolCall, ToolName: "later-tie", Outcome: "succeeded", DurationMs: "500"},
		{EventType: TypeToolCall, ToolName: "no-outcome", DurationMs: "999999999999999999"},
		{EventType: TypeToolCall, ToolName: "nineteen-digits", Outcome: "succeeded", DurationMs: "1000000000000000000"},
		{EventType: TypeToolCall, ToolName: "negative", Outcome: "succeeded", DurationMs: "-5"},
		{EventType: TypeToolCall, ToolName: "exponent", Outcome: "succeeded", DurationMs: "1e+100"},
	}

	got := foldGeneral(rows).summary.ToolCalls

	if got.Total != 7 || got.Succeeded != 5 || got.Failed != 2 {
		t.Errorf("counts = %+v, want 7 total, 5 succeeded, 2 failed", got)
	}
	if got.SlowestMS != 999999999999999999 || got.SlowestName != "no-outcome" {
		t.Errorf("slowest = %s %d", got.SlowestName, got.SlowestMS)
	}
	if want := int64(999999999999999999 + 1300); got.TotalMS != want {
		t.Errorf("total duration = %d, want %d", got.TotalMS, want)
	}

	tie := foldGeneral(rows[:3]).summary.ToolCalls
	if tie.SlowestName != "read" {
		t.Errorf("slowest of a tie = %q, want the earlier %q", tie.SlowestName, "read")
	}
	saturated := foldGeneral(repeated(10, rows[3])).summary.ToolCalls
	if saturated.TotalMS != math.MaxInt64 {
		t.Errorf("total duration = %d, want saturated", saturated.TotalMS)
	}
	if none := foldGeneral(rows[5:]).summary.ToolCalls; none.SlowestName != "" || none.SlowestMS != 0 {
		t.Errorf("no valid duration still named a slowest tool: %+v", none)
	}
}

func TestTheFinalOutputIsTheLastFinalOneAndNotALaterIntermediateOne(t *testing.T) {
	rows := []factRow{
		{EventType: TypeAgentOutput, Kind: outputKindFinal, Seq: 1},
		{EventType: TypeAgentOutput, Kind: outputKindFinal, Seq: 2},
		{EventType: TypeAgentOutput, Kind: "intermediate", Seq: 3},
	}
	if got := foldGeneral(rows).finalOutput; got == nil || got.Seq != 2 {
		t.Fatalf("final output = %+v, want seq 2", got)
	}
	if got := foldGeneral(rows[2:]).finalOutput; got != nil {
		t.Fatalf("an intermediate output was taken as final: %+v", got)
	}
}

func TestUsagePrefersAValidRunTotalAndOtherwiseSumsTheRest(t *testing.T) {
	attempt := func(model, in, out, cost string) factRow {
		return factRow{EventType: TypeUsage, Scope: "attempt", Model: model, InputTokens: in, OutputTokens: out, CostUsd: cost, CostSource: "gateway"}
	}
	runTotal := func(in, out, cost string) factRow {
		return factRow{EventType: TypeUsage, Scope: usageScopeRunTotal, Model: "run-model", InputTokens: in, OutputTokens: out, CostUsd: cost, CostSource: "harness"}
	}
	sum := []factRow{attempt("a", "1000", "100", "0.1"), attempt("b", "200", "x", "0.2")}
	for _, tc := range []struct {
		name            string
		rows            []factRow
		in, out         int64
		cost            *float64
		model, costFrom string
	}{
		{"attempts are summed and the last one names the model", sum, 1200, 100, ptr(0.3), "b", "gateway"},
		{"a valid run total replaces the sums", append(sum, runTotal("27042", "1180", "0.9")), 27042, 1180, ptr(0.9), "run-model", "harness"},
		{"an unreadable run total falls back to the sums field by field", append(sum, runTotal("lots", "7", "")), 1200, 7, ptr(0.3), "run-model", "harness"},
		{"an earlier run total never enters the fallback sums", append(append([]factRow{}, sum...), runTotal("500", "60", "0.4"), runTotal("lots", "x", "")), 1200, 100, ptr(0.3), "run-model", "harness"},
		{"no readable cost anywhere is no cost", []factRow{attempt("a", "1", "1", ""), runTotal("1", "1", "0.1234567890123")}, 1, 1, nil, "run-model", "harness"},
		{"thirteen integer digits are not a cost", []factRow{attempt("a", "1", "1", "1000000000000")}, 1, 1, nil, "a", "gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fold := foldGeneral(tc.rows)
			u := fold.summary.Usage
			if u == nil || u.InputTokens != tc.in || u.OutputTokens != tc.out || u.Model != tc.model || u.CostSource != tc.costFrom {
				t.Fatalf("usage = %+v, want %d/%d from %s via %s", u, tc.in, tc.out, tc.model, tc.costFrom)
			}
			if (fold.costUSD == nil) != (tc.cost == nil) || (tc.cost != nil && *fold.costUSD != *tc.cost) {
				t.Fatalf("cost = %v, want %v", deref(fold.costUSD), deref(tc.cost))
			}
		})
	}
	if fold := foldGeneral([]factRow{{EventType: TypeToolCall}}); fold.summary.Usage != nil || fold.costUSD != nil {
		t.Fatalf("a run with no usage events reported usage %+v", fold.summary.Usage)
	}
}

func ptr(v float64) *float64 { return &v }

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
