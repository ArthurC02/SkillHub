package trace

import (
	"math"
	"math/big"
	"regexp"
	"strconv"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	generalSkillsShown = 100
	generalErrorsShown = 100

	outputKindFinal    = "final"
	usageScopeRunTotal = "run_total"
	toolCallSucceeded  = "succeeded"
)

var (
	reportedCount = regexp.MustCompile(`^[0-9]{1,18}$`)
	reportedUSD   = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,12})?$`)
)

func generalEventTypes() []string {
	return []string{TypeSkillActivation, TypeResourceRead, TypeToolCall, TypeError, TypeAgentOutput, TypeUsage}
}

type generalFold struct {
	summary     Summary
	costUSD     *float64
	finalOutput *gen.ListTraceGeneralFactsRow
}

func foldGeneral(rows []gen.ListTraceGeneralFactsRow) generalFold {
	fold := generalFold{summary: Summary{Skills: []SkillUse{}, Errors: []ErrorSummary{}}}
	var usage []gen.ListTraceGeneralFactsRow
	for i := range rows {
		row := &rows[i]
		switch row.EventType {
		case TypeSkillActivation:
			fold.summary.SkillsTotal++
			if len(fold.summary.Skills) < generalSkillsShown {
				fold.summary.Skills = append(fold.summary.Skills, SkillUse{Name: row.SkillName, Decision: row.Decision, Reason: row.Reason})
			}
		case TypeResourceRead:
			fold.summary.ResourceRead++
		case TypeToolCall:
			fold.summary.ToolCalls.add(row.ToolName, row.Outcome, reportedCountOf(row.DurationMs))
		case TypeError:
			fold.summary.ErrorsTotal++
			if len(fold.summary.Errors) < generalErrorsShown {
				fold.summary.Errors = append(fold.summary.Errors, ErrorSummary{Category: row.Category, Code: row.Code, Message: row.Message})
			}
		case TypeAgentOutput:
			if row.Kind == outputKindFinal {
				fold.finalOutput = row
			}
		case TypeUsage:
			usage = append(usage, *row)
		}
	}
	fold.summary.Usage, fold.costUSD = foldUsage(usage)
	return fold
}

func (t *ToolCallSummary) add(name, outcome string, durationMS int64) {
	t.Total++
	if outcome == toolCallSucceeded {
		t.Succeeded++
	} else {
		t.Failed++
	}
	t.TotalMS = saturatingAdd(t.TotalMS, durationMS)
	if durationMS > t.SlowestMS {
		t.SlowestMS, t.SlowestName = durationMS, name
	}
}

func foldUsage(rows []gen.ListTraceGeneralFactsRow) (*UsageSummary, *float64) {
	if len(rows) == 0 {
		return nil, nil
	}
	last := rows[len(rows)-1]
	summary := &UsageSummary{Model: last.Model, CostSource: last.CostSource}
	var runTotal *gen.ListTraceGeneralFactsRow
	var costSum *big.Rat
	for i := range rows {
		row := &rows[i]
		if row.Scope == usageScopeRunTotal {
			runTotal = row
			continue
		}
		summary.InputTokens = saturatingAdd(summary.InputTokens, reportedCountOf(row.InputTokens))
		summary.OutputTokens = saturatingAdd(summary.OutputTokens, reportedCountOf(row.OutputTokens))
		if cost, ok := reportedUSDOf(row.CostUsd); ok {
			if costSum == nil {
				costSum = new(big.Rat)
			}
			costSum.Add(costSum, cost)
		}
	}
	if runTotal != nil {
		if reportedCount.MatchString(runTotal.InputTokens) {
			summary.InputTokens = reportedCountOf(runTotal.InputTokens)
		}
		if reportedCount.MatchString(runTotal.OutputTokens) {
			summary.OutputTokens = reportedCountOf(runTotal.OutputTokens)
		}
		if cost, ok := reportedUSDOf(runTotal.CostUsd); ok {
			costSum = cost
		}
	}
	if costSum == nil {
		return summary, nil
	}
	usd, _ := costSum.Float64()
	return summary, &usd
}

func reportedCountOf(s string) int64 {
	if !reportedCount.MatchString(s) {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func reportedUSDOf(s string) (*big.Rat, bool) {
	if !reportedUSD.MatchString(s) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}

func saturatingAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
