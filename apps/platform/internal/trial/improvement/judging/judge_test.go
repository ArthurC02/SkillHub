package judging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/llmadapter"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

const (
	eventID     = "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01"
	finalOutput = "結果 ✅：Removed 17 duplicate rows and saved the result to output.xlsx."
)

func fixtureMaterial(complete bool) (facts.Material, map[string]trace.EventView) {
	event := trace.EventView{
		EventID:    eventID,
		Attempt:    1,
		Seq:        3,
		OccurredAt: "2026-08-17T09:12:07Z",
		EmittedBy:  trace.SourceSandbox,
		Type:       trace.TypeToolCall,
		Payload:    json.RawMessage(`{"tool_name":"bash","outcome":"succeeded","duration_ms":3412}`),
	}
	m := facts.Material{
		Criteria: []testlab.Criterion{
			{ID: "c1", Text: "the duplicates are removed"},
			{ID: "c2", Text: "an xlsx file is produced"},
		},
		Advanced: trace.AdvancedView{Complete: complete, Events: []trace.EventView{event}},
		Summary:  trace.Summary{FinalOutput: finalOutput},
		Artifacts: []facts.Artifact{
			{FileName: "output.xlsx", SizeBytes: 4096, ContentType: "application/vnd.ms-excel", ContentHash: "sha256:abc"},
		},
	}
	return m, map[string]trace.EventView{eventID: event}
}

func strp(s string) *string { return &s }

func TestResultOutsideTheValueDomainReadsAsUndetermined(t *testing.T) {
	for _, in := range []string{"", "PASSED", "probably", "met", "true"} {
		if got := normaliseResult(in); got != verdict.ResultUndetermined {
			t.Errorf("normaliseResult(%q) = %q, want %q", in, got, verdict.ResultUndetermined)
		}
	}
	for _, in := range []string{verdict.ResultPassed, verdict.ResultFailed, verdict.ResultUndetermined} {
		if got := normaliseResult(in); got != in {
			t.Errorf("normaliseResult(%q) = %q, want it unchanged", in, got)
		}
	}
}

type verifyCase struct {
	name    string
	ref     modelport.Citation
	wantErr bool
	check   func(*testing.T, verdict.EvidenceRef)
}

func assertTraceEventCarriesIDAndOccurredAt(t *testing.T, got verdict.EvidenceRef) {
	if got.TraceEventID != eventID || got.OccurredAt == "" {
		t.Errorf("a trace ref must carry both id and occurred_at: %+v", got)
	}
}

func assertArtifactHasNoByteRangeAndReportsManifestLine(t *testing.T, got verdict.EvidenceRef) {
	if got.ByteRange != nil {
		t.Error("no artifact bytes were sent, so there is no byte range to report")
	}
	if !strings.Contains(got.Excerpt, "4096") {
		t.Errorf("the excerpt should be the platform's own manifest line, got %q", got.Excerpt)
	}
}

func assertAgentOutputCharRangeSelectsTheQuote(t *testing.T, got verdict.EvidenceRef) {
	if got.CharRange == nil {
		t.Fatal("Go computes the char range itself; it must be present")
	}
	quote := "✅：Removed 17 duplicate rows"
	if string([]rune(finalOutput)[got.CharRange.Start:got.CharRange.End]) != quote {
		t.Errorf("char range %+v does not select the quote", got.CharRange)
	}
}

func assertVerifyOutcome(t *testing.T, c verifyCase, m facts.Material, digest map[string]trace.EventView) {
	t.Helper()
	got, why := verify(c.ref, m, digest)
	if c.wantErr {
		if why == "" {
			t.Fatalf("expected the reference to be refused, got %+v", got)
		}
		return
	}
	if why != "" {
		t.Fatalf("expected the reference to resolve, got %q", why)
	}
	if !got.Available {
		t.Error("a reference that just resolved is available")
	}
	if c.check != nil {
		c.check(t, got)
	}
}

func TestVerifyResolvesOnlyReferencesThePlatformCanFind(t *testing.T) {
	m, digest := fixtureMaterial(true)

	cases := []verifyCase{
		{
			name: "a trace event in the digest, quoted correctly",
			ref: modelport.Citation{
				Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID), Quote: `"tool_name":"bash"`,
			},
			check: assertTraceEventCarriesIDAndOccurredAt,
		},
		{
			name: "an event id the model invented",
			ref: modelport.Citation{
				Kind: verdict.KindTraceEvent, TraceEventID: strp("11111111-1111-4111-8111-111111111111"),
			},
			wantErr: true,
		},
		{
			name: "a real event with a quote that is not in it",
			ref: modelport.Citation{
				Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID), Quote: "the skill worked perfectly",
			},
			wantErr: true,
		},
		{
			name:  "an artifact that is in the manifest",
			ref:   modelport.Citation{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")},
			check: assertArtifactHasNoByteRangeAndReportsManifestLine,
		},
		{
			name:    "an artifact nobody produced",
			ref:     modelport.Citation{Kind: verdict.KindArtifact, ArtifactPath: strp("report.pdf")},
			wantErr: true,
		},
		{
			name:  "a quote from the agent's final output",
			ref:   modelport.Citation{Kind: verdict.KindAgentOutput, Quote: "✅：Removed 17 duplicate rows"},
			check: assertAgentOutputCharRangeSelectsTheQuote,
		},
		{
			name:    "a quote the final output does not contain",
			ref:     modelport.Citation{Kind: verdict.KindAgentOutput, Quote: "I could not do it"},
			wantErr: true,
		},
		{
			name:    "a kind this platform has never heard of",
			ref:     modelport.Citation{Kind: "database_row", Quote: "x"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertVerifyOutcome(t, c, m, digest)
		})
	}
}

func TestAVerbatimQuoteIsRecordedAsAnExactMatch(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{
		Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows",
	}, m, digest)

	if why != "" {
		t.Fatalf("a verbatim quote resolves: %q", why)
	}
	if got.Match != verdict.MatchExact {
		t.Errorf("match = %q, want %q", got.Match, verdict.MatchExact)
	}
	if got.ReattributedFrom != "" {
		t.Errorf("the judge filed this one correctly, so nothing was re-attributed: %q", got.ReattributedFrom)
	}
	if got.CharRange == nil {
		t.Fatal("an exact hit keeps its offset; that is what the fast path is for")
	}
}

func TestAQuoteThatOnlyMatchesAfterNormalisationSaysSo(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{

		Kind: verdict.KindAgentOutput, Quote: "Removed 17  duplicate rows and saved}],",
	}, m, digest)

	if why != "" {
		t.Fatalf("a quote that is verbatim correct but for its wrapping resolves: %q", why)
	}
	if got.Match != verdict.MatchNormalized {
		t.Errorf("match = %q, want %q: a widened comparison is an auditable fact, not a silent success",
			got.Match, verdict.MatchNormalized)
	}
	if got.Excerpt != "Removed 17 duplicate rows and saved" {
		t.Errorf("the excerpt is the string that was actually verified, got %q", got.Excerpt)
	}
	if got.CharRange != nil {
		t.Error("the offset found was into a normalised copy; reporting it would point the reader at nothing")
	}
}

func TestAQuoteFiledUnderTheWrongSourceIsCorrectedInsteadOfRefused(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{
		Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx"), Quote: `"tool_name":"bash"`,
	}, m, digest)

	if why != "" {
		t.Fatalf("the quote is in this run's trace, so the citation holds: %q", why)
	}
	if got.Kind != verdict.KindTraceEvent || got.TraceEventID != eventID {
		t.Errorf("kind is corrected to where the quote actually is, got %+v", got)
	}
	if got.ReattributedFrom != verdict.KindArtifact {
		t.Errorf("reattributed_from = %q, want %q: the report has to show the label was wrong",
			got.ReattributedFrom, verdict.KindArtifact)
	}
	if got.Match != verdict.MatchExact {
		t.Errorf("match = %q, want %q", got.Match, verdict.MatchExact)
	}
}

func TestAFabricatedQuoteIsNotEvidenceWhateverItWasFiledAs(t *testing.T) {
	m, digest := fixtureMaterial(true)
	ref := modelport.Citation{
		Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx"),
		Quote: "the duplicates were removed by hand",
	}

	got, why := verify(ref, m, digest)
	if why != "" {
		t.Fatalf("the manifest row is still checkable, so the citation is kept: %q", why)
	}

	if got.Match != verdict.MatchNotChecked {
		t.Errorf("match = %q, want %q: nothing verified this quote, and nothing looked either",
			got.Match, verdict.MatchNotChecked)
	}

	weight := 2.0
	m.Rubric = &testlab.Rubric{Version: "v1", Items: []testlab.RubricItem{
		{ID: "c1", Text: "quote the sentence that shows it", Weight: &weight, EvidenceRequired: true},
		{ID: "c2", Text: "an xlsx file is produced", EvidenceRequired: false},
	}}
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "it says so",
				Citations: []modelport.Citation{ref}},
			{CriterionID: "c2", Result: verdict.ResultPassed, Reason: "the file is there",
				Citations: []modelport.Citation{
					{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")}}},
		},
	}, digest, evidenceCuts{})

	if results[0].Result != verdict.ResultUndetermined {
		t.Errorf("an item requiring evidence cannot be passed on a quote nothing verified: %+v", results[0])
	}
	if !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("the downgrade wears the label the UI reads (丙-10): %q", results[0].Reason)
	}
	if len(results[0].Evidence) != 1 {
		t.Error("the citation is still stored; the reader is shown what was offered and why it was not enough")
	}

	if results[1].Result != verdict.ResultPassed {
		t.Errorf("an item that never asked for a quote is not downgraded for lacking one: %+v", results[1])
	}
}

func TestATraceCitationWithoutAQuoteDoesNotPassAnItemRequiringOne(t *testing.T) {
	m, digest := fixtureMaterial(true)
	bare := modelport.Citation{Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID)}
	m.Rubric = &testlab.Rubric{Version: "v1", Items: []testlab.RubricItem{
		{ID: "c1", Text: "quote the command that removed them", EvidenceRequired: true},
		{ID: "c2", Text: "a tool ran", EvidenceRequired: false},
	}}
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "bash ran", Citations: []modelport.Citation{bare}},
			{CriterionID: "c2", Result: verdict.ResultPassed, Reason: "bash ran", Citations: []modelport.Citation{bare}},
		},
	}, digest, evidenceCuts{})

	if results[0].Result != verdict.ResultUndetermined || !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("an event's existence passed an item that requires a quote: %+v", results[0])
	}
	if len(results[0].Evidence) != 1 || results[0].Evidence[0].Match != verdict.MatchNotChecked {
		t.Errorf("evidence = %+v, want the event kept and marked not_checked: nothing was quoted to compare", results[0].Evidence)
	}
	if results[1].Result != verdict.ResultPassed {
		t.Errorf("an item that never asked for a quote is not downgraded for lacking one: %+v", results[1])
	}
}

func TestAShortQuoteIsNotHandedTheLoosenedComparison(t *testing.T) {
	m, digest := fixtureMaterial(true)
	short := `"Removed 17"`

	if _, why := verify(modelport.Citation{
		Kind: verdict.KindAgentOutput, Quote: short,
	}, m, digest); why == "" {
		t.Error("a quote below the length floor may only be accepted verbatim")
	}
	if n := utf8.RuneCountInString(normalizeQuote(short)); n >= minNormalizedQuote {
		t.Fatalf("this fixture stopped testing the floor: %d runes, floor is %d", n, minNormalizedQuote)
	}
	if !strings.Contains(normalizeQuote(finalOutput), normalizeQuote(short)) {
		t.Fatal("this fixture stopped testing the floor: the quote is no longer a normalised substring")
	}
}

func TestUnverifiableEvidenceDowngradesTheVerdictRatherThanBeingStored(t *testing.T) {
	m, digest := fixtureMaterial(true)
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "trust me",
				Citations: []modelport.Citation{
					{Kind: verdict.KindTraceEvent, TraceEventID: strp("22222222-2222-4222-8222-222222222222")},
				}},
			{CriterionID: "c2", Result: verdict.ResultFailed, Reason: "no xlsx",
				Citations: []modelport.Citation{
					{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")},
				}},
		},
	}, digest, evidenceCuts{})

	if len(results) != 2 {
		t.Fatalf("expected one entry per snapshot criterion, got %d", len(results))
	}
	if results[0].Result != verdict.ResultUndetermined {
		t.Errorf("a pass resting on an invented event id must not be stored as a pass: %+v", results[0])
	}
	if !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("the downgrade has to say why: %q", results[0].Reason)
	}
	if len(results[0].Evidence) != 0 {
		t.Error("a reference that did not resolve must not be stored as evidence")
	}
	if results[1].Result != verdict.ResultFailed || len(results[1].Evidence) != 1 {
		t.Errorf("a verifiable failure survives intact: %+v", results[1])
	}
	for _, r := range results {
		if r.Source != verdict.SourceModel {
			t.Errorf("everything from the judge is labelled model, got %q", r.Source)
		}
	}
}

func TestOneUnverifiableCitationDowngradesAVerdictThatAlsoCitesSomethingReal(t *testing.T) {
	m, digest := fixtureMaterial(true)
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "both of these check out",
				Citations: []modelport.Citation{

					{Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows"},

					{Kind: verdict.KindTraceEvent, TraceEventID: strp("33333333-3333-4333-8333-333333333333"),
						Quote: "the deduplicator ran twice and agreed with itself"},
				}},
			{CriterionID: "c2", Result: verdict.ResultFailed, Reason: "no xlsx",
				Citations: []modelport.Citation{
					{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")},
				}},
		},
	}, digest, evidenceCuts{})

	if results[0].Result != verdict.ResultUndetermined {
		t.Errorf("a verdict half of whose citations were invented was stored as %q", results[0].Result)
	}
	if !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("the downgrade has to name the reference it could not find: %q", results[0].Reason)
	}

	if len(results[0].Evidence) != 1 {
		t.Errorf("the verifiable citation should survive the downgrade, got %d", len(results[0].Evidence))
	}
}

func TestACriterionTheJudgeDidNotAnswerIsUndeterminedAndStillListed(t *testing.T) {
	m, digest := fixtureMaterial(true)
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "the duplicates are gone",
				Citations: []modelport.Citation{
					{Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows"},
				}},

			{CriterionID: "c99", Result: verdict.ResultPassed, Reason: "invented"},
		},
	}, digest, evidenceCuts{})

	if len(results) != 2 {
		t.Fatalf("the report has one entry per snapshot criterion, got %d", len(results))
	}
	if results[0].Result != verdict.ResultPassed {
		t.Errorf("the answered criterion keeps its verdict: %+v", results[0])
	}
	if results[1].CriterionID != "c2" || results[1].Result != verdict.ResultUndetermined {
		t.Errorf("the unanswered criterion is listed as undetermined, got %+v", results[1])
	}
	for _, r := range results {
		if r.CriterionID == "c99" {
			t.Error("a criterion id that was never sent must be dropped")
		}
	}
	if verdict.OverallFrom(results) != verdict.OverallPartiallyMet {
		t.Errorf("one pass and one unknown is partially met, got %q", verdict.OverallFrom(results))
	}
}

func TestVerdictWithoutVerifiedEvidenceIsUndetermined(t *testing.T) {
	m, digest := fixtureMaterial(true)
	results := merge(m, &modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "trust me"},
			{CriterionID: "c2", Result: verdict.ResultFailed, Reason: "also trust me"},
		},
	}, digest, evidenceCuts{})

	for _, result := range results {
		if result.Result != verdict.ResultUndetermined {
			t.Errorf("zero-evidence verdict survived: %+v", result)
		}
		if !strings.Contains(result.Reason, "no verifiable evidence") {
			t.Errorf("downgrade does not explain the evidence failure: %q", result.Reason)
		}
	}
}

func TestAPassIsRefusedWhenTheEvidenceCouldBeIncomplete(t *testing.T) {
	pass := modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{
			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "looks right",
				Citations: []modelport.Citation{{Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows"}}},
			{CriterionID: "c2", Result: verdict.ResultFailed, Reason: "no file",
				Citations: []modelport.Citation{{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")}}},
		},
	}

	incomplete, digest := fixtureMaterial(false)
	got := merge(incomplete, &pass, digest, evidenceCuts{})
	if got[0].Result != verdict.ResultUndetermined {
		t.Errorf("a trace with holes cannot support a pass (丙-1), got %q", got[0].Result)
	}
	if got[1].Result != verdict.ResultFailed {
		t.Error("a failure is not softened by missing evidence: what is there still contradicts the criterion")
	}

	complete, digest := fixtureMaterial(true)
	got = merge(complete, &pass, digest, evidenceCuts{batch: true})
	if got[0].Result != verdict.ResultUndetermined {
		t.Errorf("judging on truncated input cannot support a pass (§6.3), got %q", got[0].Result)
	}

	got = merge(complete, &pass, digest, evidenceCuts{})
	if got[0].Result != verdict.ResultPassed {
		t.Error("with complete, untruncated evidence a pass is a pass")
	}
}

func TestTheRubricIsSentOnlyForTheCriteriaTheRequestCarries(t *testing.T) {
	m, _ := fixtureMaterial(true)
	weight := 3.0
	m.Rubric = &testlab.Rubric{
		Version: "content-007/writing/v1",
		Items: []testlab.RubricItem{
			{ID: "c1", Text: "quote the sentence that shows it", Weight: &weight, EvidenceRequired: true},
			{ID: "c2", Text: "absence cannot be quoted", EvidenceRequired: false},
			{ID: "gone", Text: "strengthens a criterion this snapshot does not have"},
		},
	}

	req, sent := buildRequest(m, "")
	dropped := sent.dropped
	if req.Rubric == nil {
		t.Fatal("the snapshot's rubric has to reach the judge")
	}
	if len(req.Rubric.Items) != 2 {
		t.Fatalf("only items naming a sent criterion go out, got %+v", req.Rubric.Items)
	}
	if req.Rubric.Items[0].Weight == nil || *req.Rubric.Items[0].Weight != weight {
		t.Errorf("weight is carried through untouched, got %+v", req.Rubric.Items[0])
	}
	if !req.Rubric.Items[0].EvidenceRequired || req.Rubric.Items[1].EvidenceRequired {
		t.Error("evidence_required is per item, not a global switch")
	}
	if len(dropped) != 1 || dropped[0] != "gone" {
		t.Errorf("a dropped item must be reported so it can be warned about, got %v", dropped)
	}
}

func TestARunWithNoRubricSendsNoneAndRecordsNoVersion(t *testing.T) {
	m, _ := fixtureMaterial(true)
	req, sent := buildRequest(m, "")
	dropped := sent.dropped
	if req.Rubric != nil {
		t.Errorf("no rubric means no rubric field, got %+v", req.Rubric)
	}
	if dropped != nil {
		t.Errorf("nothing to drop, got %v", dropped)
	}
}

func TestARubricWithNothingLeftToSendIsNotRecordedAsInForce(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.Rubric = &testlab.Rubric{
		Version: "content-007/writing/v1",
		Items:   []testlab.RubricItem{{ID: "nobody", Text: "x"}},
	}
	req, sent := buildRequest(m, "")
	dropped := sent.dropped
	if req.Rubric != nil {
		t.Errorf("nothing was left to send, got %+v", req.Rubric)
	}
	if len(dropped) != 1 {
		t.Errorf("the item still has to be reported, got %v", dropped)
	}
}

func TestDigestKeepsTheTailAndSaysWhenItCut(t *testing.T) {
	view := trace.AdvancedView{Complete: true}
	for i := 0; i < maxDigestCount+5; i++ {
		view.Events = append(view.Events, trace.EventView{
			EventID: string(rune('a'+i%26)) + "-" + time.Now().Format("150405") + "-" + strings.Repeat("x", i%3),
			Type:    trace.TypeToolCall,
			Payload: json.RawMessage(`{"n":` + strings.Repeat("1", 1+i%3) + `}`),
		})
	}

	view.Events = append(view.Events, trace.EventView{
		EventID: eventID, Type: trace.TypeAgentOutput, Payload: json.RawMessage(`{"kind":"final"}`),
	})

	view.Events = append(view.Events, trace.EventView{
		EventID: "lifecycle", Type: trace.TypeRunLifecycle, Payload: json.RawMessage(`{}`),
	})

	entries, digest, cuts := buildDigest(view)
	if !cuts.DroppedEvents {
		t.Error("a digest that dropped events has to report the cut")
	}

	if cuts.TrimmedExcerpts {
		t.Error("no payload here is over maxDigestEntry; that cut must not be claimed")
	}
	if len(entries) != maxDigestCount {
		t.Errorf("digest capped at %d, got %d", maxDigestCount, len(entries))
	}
	if _, ok := digest[eventID]; !ok {
		t.Error("the tail is what is kept; the last citable event is missing")
	}
	if _, ok := digest["lifecycle"]; ok {
		t.Error("run_lifecycle is not a citable event: it is not a source of truth (iron rule 5)")
	}
}

func TestDigestReportsAnExcerptCutAsTruncation(t *testing.T) {
	view := trace.AdvancedView{Complete: true, Events: []trace.EventView{{
		EventID: eventID, Type: trace.TypeAgentOutput,
		Payload: json.RawMessage(`{"text":"` + strings.Repeat("x", maxDigestEntry) + `"}`),
	}}}
	entries, _, cuts := buildDigest(view)
	if !cuts.TrimmedExcerpts {
		t.Fatal("cutting one trace payload was not reported as truncation")
	}

	if cuts.DroppedEvents {
		t.Error("one event is not over maxDigestCount; no event was dropped")
	}
	if len([]rune(entries[0].Excerpt)) != maxDigestEntry {
		t.Fatalf("excerpt length = %d, want %d", len([]rune(entries[0].Excerpt)), maxDigestEntry)
	}
}

func fakeJudge(t *testing.T, status int, body any, capture *llmclient.JudgeRunRequest) *llmclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/judge-run" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if capture != nil {
			if err := json.NewDecoder(r.Body).Decode(capture); err != nil {
				t.Errorf("request body: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

func TestJudgeRunSendsTheContractShapeAndReturnsTheVerdict(t *testing.T) {
	var got llmclient.JudgeRunRequest
	client := fakeJudge(t, http.StatusOK, llmclient.JudgeRunResponse{
		Verdict: llmclient.JudgeVerdict{
			CriterionResults: []llmclient.CriterionVerdict{
				{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "done", EvidenceRefs: nil},
			},
			Overall: string(verdict.OverallMet), Summary: "the task was completed",
		},
		Model: "gpt-6-sol", PromptVersion: "judge-run@2026-08-17",
	}, &got)

	resp, err := client.JudgeRun(context.Background(), llmclient.JudgeRunRequest{
		RunID: "r", EvaluationID: "e", UserPrompt: "dedupe this",
		Criteria:    []llmclient.JudgeCriterion{{ID: "c1", Text: "duplicates removed"}},
		FinalOutput: finalOutput,
		Artifacts:   []llmclient.JudgeArtifact{},
		TraceDigest: llmclient.TraceDigest{Complete: true, Entries: []llmclient.TraceDigestEntry{}},
		Truncation:  []string{},
	})
	if err != nil {
		t.Fatalf("judge call: %v", err)
	}
	if resp.Model != "gpt-6-sol" || resp.PromptVersion == "" {
		t.Errorf("the caller stores what actually judged, got %+v", resp)
	}

	if got.FinalOutput == "" || got.TraceDigest.Entries == nil || got.Truncation == nil {
		t.Errorf("every field the contract lists is sent, got %+v", got)
	}
}

func TestAJudgeFailureIsAnErrorAndNotALenientVerdict(t *testing.T) {
	client := fakeJudge(t, http.StatusBadGateway, map[string]string{
		"error": "the model returned a result outside the value domain",
	}, nil)
	if _, err := client.JudgeRun(context.Background(), llmclient.JudgeRunRequest{}); err == nil {
		t.Fatal("a 502 from the judge must surface as an error")
	}

	svc := Judges{Judge: llmadapter.JudgeOrNone(client)}
	if _, err := svc.Assess(context.Background(), facts.Material{
		Criteria: []testlab.Criterion{{ID: "c1", Text: "x"}},
	}, "", 0); err == nil {
		t.Fatal("judge() propagates the failure so Evaluate can record status=failed")
	}
}

func TestAJudgeCallCostsOnlyWhatTheGatewayReported(t *testing.T) {
	cost := 0.0042
	for _, tc := range []struct {
		name   string
		source llmclient.CostSource
		want   *float64
	}{
		{"priced by the gateway", llmclient.CostSourceGateway, &cost},
		{"priced by something other than the gateway", "estimated", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeJudge(t, http.StatusOK, llmclient.JudgeRunResponse{
				Verdict: llmclient.JudgeVerdict{
					CriterionResults: []llmclient.CriterionVerdict{
						{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "done"},
					},
					Overall: string(verdict.OverallMet),
				},
				Usage: &llmclient.GatewayUsage{PromptTokens: 10, CostUSD: &cost, CostSource: tc.source},
			}, nil)
			v, err := (Judges{Judge: llmadapter.JudgeOrNone(client)}).Assess(context.Background(), facts.Material{
				Criteria: []testlab.Criterion{{ID: "c1", Text: "x"}},
			}, "", 0)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if (v.Usage.ReportedCostUSD() == nil) != (tc.want == nil) || (v.Usage.ReportedCostUSD() != nil && *v.Usage.ReportedCostUSD() != *tc.want) {
				t.Errorf("cost = %v, want %v", v.Usage.ReportedCostUSD(), tc.want)
			}
		})
	}
}

func TestNoJudgeConfiguredIsAFailureAndNotASilentPass(t *testing.T) {
	svc := Judges{}
	if _, err := svc.Assess(context.Background(), facts.Material{}, "", 0); err == nil {
		t.Fatal("a deployment with no judge cannot produce a task verdict, and must say so")
	}
}

func TestJudgeCallHonoursTheCallersCancellation(t *testing.T) {

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	client := &llmclient.Client{BaseURL: srv.URL}
	if _, err := client.JudgeRun(ctx, llmclient.JudgeRunRequest{}); err == nil {
		t.Fatal("a cancelled context must end the call")
	}
}

func TestUnreadableOutputsReachTheJudgeAndCannotSupportAPass(t *testing.T) {
	m, digest := fixtureMaterial(true)
	m.Artifacts, m.Absent = nil, facts.ArtifactAbsence{Deleted: 1}

	req, sent := buildRequest(m, "")
	truncation := sent.truncation
	if !strings.Contains(strings.Join(req.Truncation, " "), "artifacts.unreadable") {
		t.Fatalf("the judge got an empty artifact list with nothing said about it: %v", req.Truncation)
	}

	pass := modelport.Judgement{Criteria: []modelport.CriterionVerdict{
		{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "looks right",
			Citations: []modelport.Citation{
				{Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows"},
			}},
	}}
	got := merge(m, &pass, digest, evidenceCuts{batch: batchWideCut(truncation)})
	if got[0].Result != verdict.ResultUndetermined {
		t.Errorf("a pass judged with the run's outputs unreadable was stored as a pass: %+v", got[0])
	}
}

func TestATrimmedExcerptOnlySilencesTheCriterionThatCitesIt(t *testing.T) {
	judgement := modelport.Judgement{
		Criteria: []modelport.CriterionVerdict{

			{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "the tool call shows it",
				Citations: []modelport.Citation{{Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID), Quote: `"tool_name":"bash"`}}},

			{CriterionID: "c2", Result: verdict.ResultPassed, Reason: "the file is there",
				Citations: []modelport.Citation{{Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx")}}},
		},
	}
	m, digest := fixtureMaterial(true)
	got := merge(m, &judgement, digest, evidenceCuts{trimmedEvents: map[string]bool{eventID: true}})

	if got[0].Result != verdict.ResultUndetermined {
		t.Errorf("a verdict resting on a trimmed excerpt cannot be a pass, got %q", got[0].Result)
	}
	if got[1].Result != verdict.ResultPassed {
		t.Errorf("a verdict resting on something nothing trimmed keeps its pass, got %q: %s", got[1].Result, got[1].Reason)
	}
}

func TestOnlyAnExcerptCutIsNarrow(t *testing.T) {
	if batchWideCut([]string{"trace_digest.entries[].excerpt"}) {
		t.Error("a trimmed excerpt is the one cut merge can attribute to a criterion")
	}
	for _, name := range []string{"final_output", "criteria", "artifacts", "artifacts.unreadable", "trace_events", "trace_digest.entries"} {
		if !batchWideCut([]string{name}) {
			t.Errorf("%s hides something no citation can account for; it must stay batch-wide", name)
		}
	}
	if !batchWideCut([]string{"trace_digest.entries[].excerpt", "final_output"}) {
		t.Error("one batch-wide cut among narrow ones is still batch-wide")
	}
}

func TestTheJudgeSeesAtMostTwentyCriteriaAndSaysTheListWasCut(t *testing.T) {
	for _, tc := range []struct {
		given   int
		wantCut bool
	}{
		{maxCriteria, false},
		{maxCriteria + 1, true},
	} {
		m, _ := fixtureMaterial(true)
		m.Criteria = nil
		for i := range tc.given {
			m.Criteria = append(m.Criteria, testlab.Criterion{ID: "c" + strconv.Itoa(i), Text: "criterion " + strconv.Itoa(i)})
		}

		req, _ := buildRequest(m, "")

		cut := slices.Contains(req.Truncation, "criteria")
		if len(req.Criteria) != maxCriteria || cut != tc.wantCut {
			t.Errorf("%d criteria: sent %d, cut=%t; want %d sent, cut=%t", tc.given, len(req.Criteria), cut, maxCriteria, tc.wantCut)
		}
	}
}
