package eval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

var packageTree = []string{
	"SKILL.md",
	"assets/build.py",
	"references/tiers.md",
	"references/unused.md",
	"scripts/rate.py",
	"scripts/rates.csv",
}

func event(t *testing.T, kind string, payload map[string]any) trace.EventView {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return trace.EventView{Type: kind, Payload: raw}
}

func aCompleteTraceThatReadTiersAndRanRate(t *testing.T) trace.AdvancedView {
	return trace.AdvancedView{Complete: true, Events: []trace.EventView{
		event(t, trace.TypeResourceRead, map[string]any{"resource_path": "references/tiers.md", "outcome": "read"}),
		event(t, trace.TypeToolCall, map[string]any{"tool_name": "Bash", "arguments": map[string]any{"command": "python scripts/rate.py --total 40"}}),
		event(t, trace.TypeAgentOutput, map[string]any{"text": "I did not need references/unused.md"}),
	}}
}

func TestOnlyInstructionFilesTheRunNeverTouchedAreListedAsUnopened(t *testing.T) {
	got := unopenedFiles(aCompleteTraceThatReadTiersAndRanRate(t), packageTree)
	if strings.Join(got, ",") != "references/unused.md" {
		t.Fatalf("unopened = %v, want only references/unused.md", got)
	}
}

func TestNoFileIsCalledUnopenedWhenTheTraceCannotShowEverything(t *testing.T) {
	for name, cut := range map[string]func(*trace.AdvancedView){
		"a trace with known gaps":        func(v *trace.AdvancedView) { v.Complete = false },
		"a trace with more pages":        func(v *trace.AdvancedView) { v.HasMore = true },
		"a trace cut for the evaluation": func(v *trace.AdvancedView) { v.EvaluationTruncated = true },
	} {
		t.Run(name, func(t *testing.T) {
			view := aCompleteTraceThatReadTiersAndRanRate(t)
			cut(&view)
			if got := unopenedFiles(view, packageTree); got != nil {
				t.Fatalf("unopened = %v, want nothing claimed from a partial trace", got)
			}
			if digest := withUnopenedFiles("digest\n", view, packageTree); digest != "digest\n" {
				t.Fatalf("digest = %q, want it unchanged", digest)
			}
		})
	}
}

func TestTheDigestNamesEachUnopenedFileUnderItsHeading(t *testing.T) {
	got := withUnopenedFiles("digest\n", aCompleteTraceThatReadTiersAndRanRate(t), packageTree)
	want := "digest\n" + unopenedFilesHeading + "  references/unused.md\n"
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	if every := withUnopenedFiles("digest\n", aCompleteTraceThatReadTiersAndRanRate(t), []string{"SKILL.md"}); every != "digest\n" {
		t.Fatalf("digest = %q, want it unchanged when every file was opened", every)
	}
}

func TestTheUnopenedListNeverPushesTheDigestPastItsLimit(t *testing.T) {
	full := strings.Repeat("x", maxDigestChars-5)
	got := withUnopenedFiles(full, aCompleteTraceThatReadTiersAndRanRate(t), packageTree)
	if n := len([]rune(got)); n != maxDigestChars {
		t.Fatalf("digest is %d runes, want it cut at %d", n, maxDigestChars)
	}
}

type capturingSuggester struct{ got modelport.ImprovementRequest }

func (c *capturingSuggester) SuggestImprovements(_ context.Context, req modelport.ImprovementRequest) (*modelport.Improvements, error) {
	c.got = req
	return nil, errors.New("stop before storing")
}

type packageStore map[string][]byte

func (p packageStore) Get(_ context.Context, key string) ([]byte, error) { return p[key], nil }

func TestTheImprovementModelIsToldWhichFilesTheRunNeverOpened(t *testing.T) {
	suggester := &capturingSuggester{}
	pkg := zipWithRoot(t, "", map[string]string{
		"SKILL.md":             "---\nname: s\ndescription: d\n---\nRun scripts/rate.py.\n",
		"references/tiers.md":  "tiers\n",
		"references/unused.md": "unused\n",
		"scripts/rate.py":      "print(1)\n",
	})
	s := &Service{Suggester: suggester, Store: packageStore{"pkg": pkg}}
	m := facts.Material{Advanced: aCompleteTraceThatReadTiersAndRanRate(t)}
	m.Version.PackageObjectKey = "pkg"
	v := assessment{overall: verdict.OverallNotMet, results: []verdict.CriterionResult{{
		CriterionID: "c1", Text: "uses the tier table", Result: verdict.ResultFailed, Reason: "wrong tier",
		Evidence: []verdict.EvidenceRef{{Kind: verdict.KindAgentOutput, Excerpt: citableExcerpt, Available: true}},
	}}}

	if got := s.suggest(context.Background(), m, gen.Evaluation{}, v); got != SuggestionsUnavailable {
		t.Fatalf("outcome = %q, want the capturing suggester's refusal", got)
	}
	if !strings.HasSuffix(suggester.got.EvaluationDigest, unopenedFilesHeading+"  references/unused.md\n") {
		t.Fatalf("digest does not end with the unopened file:\n%s", suggester.got.EvaluationDigest)
	}
}

func toolCallTrace(t *testing.T, complete bool, calls int, args string) facts.Material {
	view := trace.AdvancedView{Complete: complete, Events: []trace.EventView{}}
	for i := 0; i < calls; i++ {
		view.Events = append(view.Events, event(t, trace.TypeToolCall, map[string]any{
			"tool_name": "Bash", "outcome": "succeeded", "duration_ms": 7, "arguments": map[string]any{"command": args},
		}))
	}
	return facts.Material{Advanced: view}
}

func toolCallSection(digest string) string {
	_, after, _ := strings.Cut(digest, toolCallsHeading)
	return after
}

func TestTheDigestListsEachToolCallInOrderWithItsOutcomeAndArguments(t *testing.T) {
	m := facts.Material{Advanced: trace.AdvancedView{Complete: true, Events: []trace.EventView{
		event(t, trace.TypeToolCall, map[string]any{"tool_name": "Bash", "outcome": "failed", "duration_ms": 40, "arguments": map[string]any{"command": "python scripts/rate.py"}}),
		event(t, trace.TypeToolCall, map[string]any{"tool_name": "Read", "outcome": "succeeded", "duration_ms": 3, "arguments": nil}),
	}}}

	digest, _ := suggestionDigest(m, assessment{})

	want := "  1. Bash -> failed in 40 ms: {\"command\":\"python scripts/rate.py\"}\n  2. Read -> succeeded in 3 ms\n"
	if got := toolCallSection(digest); got != want {
		t.Fatalf("tool call section = %q, want %q", got, want)
	}
}

func TestTheDigestSaysWhenTheRunMadeNoToolCalls(t *testing.T) {
	digest, _ := suggestionDigest(toolCallTrace(t, true, 0, ""), assessment{})
	if got := toolCallSection(digest); got != "  (none)\n" {
		t.Fatalf("tool call section = %q, want only (none)", got)
	}
}

func TestTheDigestWarnsThatToolCallsMayBeMissingFromAPartialTrace(t *testing.T) {
	digest, _ := suggestionDigest(toolCallTrace(t, false, 0, ""), assessment{})
	if got := toolCallSection(digest); got != toolCallsPartialNotice+"  (none)\n" {
		t.Fatalf("tool call section = %q, want the partial notice before (none)", got)
	}
}

func TestTheDigestShowsAtMostItsLimitOfToolCallsAndCountsTheRest(t *testing.T) {
	atLimit, _ := suggestionDigest(toolCallTrace(t, true, 60, "ls"), assessment{})
	if section := toolCallSection(atLimit); strings.Count(section, "Bash") != 60 || strings.Contains(section, "more calls not shown") {
		t.Fatalf("60 calls: section = %q, want all 60 and no omission line", section)
	}
	overLimit, _ := suggestionDigest(toolCallTrace(t, true, 61, "ls"), assessment{})
	section := toolCallSection(overLimit)
	if strings.Count(section, "Bash") != 60 || !strings.HasSuffix(section, "  ... 1 more calls not shown\n") {
		t.Fatalf("61 calls: section = %q, want 60 calls then one omitted", section)
	}
}

func TestTheDigestCutsEachToolCallsArgumentsToItsRuneBudget(t *testing.T) {
	long := strings.Repeat("字", 400)
	digest, _ := suggestionDigest(toolCallTrace(t, true, 1, long), assessment{})
	line := strings.TrimSuffix(toolCallSection(digest), "\n")
	_, args, _ := strings.Cut(line, ": ")
	if n := len([]rune(args)); n != 300 {
		t.Fatalf("arguments are %d runes, want them cut at 300", n)
	}
}
