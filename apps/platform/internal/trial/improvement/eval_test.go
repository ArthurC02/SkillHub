package eval

// Unit coverage for the part of EVAL-001 that decides what gets stored: the
// downgrade rules of ADR-026 defence 3, the digest that bounds both cost and the
// citation set, and the wire call to apps/llm.
//
// The judge here is an httptest server speaking the llm-internal.yaml shapes, not
// the real Python service: what is under test is the caller's half - deadline,
// failure handling, and the fact that nothing a model returns is trusted without
// being re-resolved first.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// --- fixtures ----------------------------------------------------------------

const (
	eventID     = "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01"
	finalOutput = "結果 ✅：Removed 17 duplicate rows and saved the result to output.xlsx."
)

func fixtureMaterial(complete bool) (material, map[string]trace.EventView) {
	event := trace.EventView{
		EventID:    eventID,
		Attempt:    1,
		Seq:        3,
		OccurredAt: "2026-08-17T09:12:07Z",
		EmittedBy:  trace.SourceSandbox,
		Type:       trace.TypeToolCall,
		Payload:    json.RawMessage(`{"tool_name":"bash","outcome":"succeeded","duration_ms":3412}`),
	}
	m := material{
		criteria: []testlab.Criterion{
			{ID: "c1", Text: "the duplicates are removed"},
			{ID: "c2", Text: "an xlsx file is produced"},
		},
		advanced: trace.AdvancedView{Complete: complete, Events: []trace.EventView{event}},
		summary:  trace.Summary{FinalOutput: finalOutput},
		artifacts: []ArtifactFacts{
			{FileName: "output.xlsx", SizeBytes: 4096, ContentType: "application/vnd.ms-excel", ContentHash: "sha256:abc"},
		},
	}
	return m, map[string]trace.EventView{eventID: event}
}

func strp(s string) *string { return &s }

func TestRunReadersFailClosedAndHideMissingRuns(t *testing.T) {
	ctx := context.Background()
	var svc Service
	if _, err := svc.runFacts(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("run facts without owner reader: %v", err)
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("evaluation input without owner reader: %v", err)
	}

	svc.ReadRunFacts = func(context.Context, pgtype.UUID, pgtype.UUID) (RunFacts, bool, error) {
		return RunFacts{}, false, nil
	}
	if _, err := svc.runFacts(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing run facts: %v", err)
	}
	svc.ReadEvaluationInput = func(context.Context, pgtype.UUID, pgtype.UUID) (EvaluationInput, bool, error) {
		return EvaluationInput{}, false, nil
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRegistryReadNotConfigured) {
		t.Errorf("evaluation input without Registry readers: %v", err)
	}
	svc.ReadVersion = func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
		return VersionFacts{}, false, nil
	}
	svc.ReadSkill = func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
		return SkillFacts{}, false, nil
	}
	svc.ReadRuntimeCompatibility = func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error) {
		return RuntimeCompatibility{}, false, nil
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing evaluation input: %v", err)
	}
}

// --- the value domain (ADR-026 defence 1) ------------------------------------

func TestResultOutsideTheValueDomainReadsAsUndetermined(t *testing.T) {
	for _, in := range []string{"", "PASSED", "probably", "met", "true"} {
		if got := normaliseResult(in); got != ResultUndetermined {
			t.Errorf("normaliseResult(%q) = %q, want %q", in, got, ResultUndetermined)
		}
	}
	for _, in := range []string{ResultPassed, ResultFailed, ResultUndetermined} {
		if got := normaliseResult(in); got != in {
			t.Errorf("normaliseResult(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestOverallIsRecomputedFromTheStoredCriteria(t *testing.T) {
	res := func(results ...string) []CriterionResult {
		out := make([]CriterionResult, 0, len(results))
		for i, r := range results {
			out = append(out, CriterionResult{CriterionID: string(rune('a' + i)), Result: r})
		}
		return out
	}
	cases := []struct {
		name string
		in   []CriterionResult
		want string
	}{
		{"no criteria at all", nil, OverallUndetermined},
		{"all passed", res(ResultPassed, ResultPassed), OverallMet},
		{"all failed", res(ResultFailed, ResultFailed), OverallNotMet},
		{"some of each", res(ResultPassed, ResultFailed), OverallPartiallyMet},
		{"a pass and an unknown", res(ResultPassed, ResultUndetermined), OverallPartiallyMet},
		{"a failure and an unknown, nothing passed", res(ResultFailed, ResultUndetermined), OverallNotMet},
		{"nothing but unknowns", res(ResultUndetermined, ResultUndetermined), OverallUndetermined},
	}
	for _, c := range cases {
		if got := overallFrom(c.in); got != c.want {
			t.Errorf("%s: overallFrom = %q, want %q", c.name, got, c.want)
		}
	}
}

// --- evidence re-verification (ADR-026 defence 3) -----------------------------

func TestVerifyResolvesOnlyReferencesThePlatformCanFind(t *testing.T) {
	m, digest := fixtureMaterial(true)
	quote := "✅：Removed 17 duplicate rows"

	cases := []struct {
		name    string
		ref     llmclient.JudgeEvidenceRef
		wantErr bool
		check   func(*testing.T, EvidenceRef)
	}{
		{
			name: "a trace event in the digest, quoted correctly",
			ref: llmclient.JudgeEvidenceRef{
				Kind: KindTraceEvent, TraceEventID: strp(eventID), Quote: `"tool_name":"bash"`,
			},
			check: func(t *testing.T, got EvidenceRef) {
				if got.TraceEventID != eventID || got.OccurredAt == "" {
					t.Errorf("a trace ref must carry both id and occurred_at: %+v", got)
				}
			},
		},
		{
			name: "an event id the model invented",
			ref: llmclient.JudgeEvidenceRef{
				Kind: KindTraceEvent, TraceEventID: strp("11111111-1111-4111-8111-111111111111"),
			},
			wantErr: true,
		},
		{
			name: "a real event with a quote that is not in it",
			ref: llmclient.JudgeEvidenceRef{
				Kind: KindTraceEvent, TraceEventID: strp(eventID), Quote: "the skill worked perfectly",
			},
			wantErr: true,
		},
		{
			name: "an artifact that is in the manifest",
			ref:  llmclient.JudgeEvidenceRef{Kind: KindArtifact, ArtifactPath: strp("output.xlsx")},
			check: func(t *testing.T, got EvidenceRef) {
				if got.ByteRange != nil {
					t.Error("no artifact bytes were sent, so there is no byte range to report")
				}
				if !strings.Contains(got.Excerpt, "4096") {
					t.Errorf("the excerpt should be the platform's own manifest line, got %q", got.Excerpt)
				}
			},
		},
		{
			name:    "an artifact nobody produced",
			ref:     llmclient.JudgeEvidenceRef{Kind: KindArtifact, ArtifactPath: strp("report.pdf")},
			wantErr: true,
		},
		{
			name: "a quote from the agent's final output",
			ref:  llmclient.JudgeEvidenceRef{Kind: KindAgentOutput, Quote: quote},
			check: func(t *testing.T, got EvidenceRef) {
				if got.CharRange == nil {
					t.Fatal("Go computes the char range itself; it must be present")
				}
				if string([]rune(finalOutput)[got.CharRange.Start:got.CharRange.End]) != quote {
					t.Errorf("char range %+v does not select the quote", got.CharRange)
				}
			},
		},
		{
			name:    "a quote the final output does not contain",
			ref:     llmclient.JudgeEvidenceRef{Kind: KindAgentOutput, Quote: "I could not do it"},
			wantErr: true,
		},
		{
			name:    "a kind this platform has never heard of",
			ref:     llmclient.JudgeEvidenceRef{Kind: "database_row", Quote: "x"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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
		})
	}
}

// --- content-first citation (ADR-043) -----------------------------------------

// The ordinary case, and the one the three-state field has to keep saying out
// loud: nothing was widened to accept this.
func TestAVerbatimQuoteIsRecordedAsAnExactMatch(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(llmclient.JudgeEvidenceRef{
		Kind: KindAgentOutput, Quote: "Removed 17 duplicate rows",
	}, m, digest)

	if why != "" {
		t.Fatalf("a verbatim quote resolves: %q", why)
	}
	if got.Match != MatchExact {
		t.Errorf("match = %q, want %q", got.Match, MatchExact)
	}
	if got.ReattributedFrom != "" {
		t.Errorf("the judge filed this one correctly, so nothing was re-attributed: %q", got.ReattributedFrom)
	}
	if got.CharRange == nil {
		t.Fatal("an exact hit keeps its offset; that is what the fast path is for")
	}
}

// G8, the failure that put 45 correct verdicts on the floor in the A round: the
// quote is right and the model wrote its own JSON delimiters into the end of it.
// It resolves now, and the report says it had to be normalised to get there.
func TestAQuoteThatOnlyMatchesAfterNormalisationSaysSo(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(llmclient.JudgeEvidenceRef{
		// Two stray spaces in the middle and the `}],` G8 was lost to.
		Kind: KindAgentOutput, Quote: "Removed 17  duplicate rows and saved}],",
	}, m, digest)

	if why != "" {
		t.Fatalf("a quote that is verbatim correct but for its wrapping resolves: %q", why)
	}
	if got.Match != MatchNormalized {
		t.Errorf("match = %q, want %q: a widened comparison is an auditable fact, not a silent success",
			got.Match, MatchNormalized)
	}
	if got.Excerpt != "Removed 17 duplicate rows and saved" {
		t.Errorf("the excerpt is the string that was actually verified, got %q", got.Excerpt)
	}
	if got.CharRange != nil {
		t.Error("the offset found was into a normalised copy; reporting it would point the reader at nothing")
	}
}

// The A round's real finding: the model had read the trace and written `artifact`
// on it. Mis-filed and fabricated are different failures, and one string search
// tells them apart.
func TestAQuoteFiledUnderTheWrongSourceIsCorrectedInsteadOfRefused(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(llmclient.JudgeEvidenceRef{
		Kind: KindArtifact, ArtifactPath: strp("output.xlsx"), Quote: `"tool_name":"bash"`,
	}, m, digest)

	if why != "" {
		t.Fatalf("the quote is in this run's trace, so the citation holds: %q", why)
	}
	if got.Kind != KindTraceEvent || got.TraceEventID != eventID {
		t.Errorf("kind is corrected to where the quote actually is, got %+v", got)
	}
	if got.ReattributedFrom != KindArtifact {
		t.Errorf("reattributed_from = %q, want %q: the report has to show the label was wrong",
			got.ReattributedFrom, KindArtifact)
	}
	if got.Match != MatchExact {
		t.Errorf("match = %q, want %q", got.Match, MatchExact)
	}
}

// G7. The citation is kept — the file really is on the manifest — but it proves
// existence and nothing else, so it cannot answer a rubric item that asked for a
// quote. That is exactly the shape 6 of the A round's 9 `passed` rested on.
func TestAFabricatedQuoteIsNotEvidenceWhateverItWasFiledAs(t *testing.T) {
	m, digest := fixtureMaterial(true)
	ref := llmclient.JudgeEvidenceRef{
		Kind: KindArtifact, ArtifactPath: strp("output.xlsx"),
		Quote: "the duplicates were removed by hand",
	}

	got, why := verify(ref, m, digest)
	if why != "" {
		t.Fatalf("the manifest row is still checkable, so the citation is kept: %q", why)
	}
	if got.Match != MatchNotFound {
		t.Errorf("match = %q, want %q: nothing verified this quote", got.Match, MatchNotFound)
	}

	weight := 2.0
	m.rubric = &testlab.Rubric{Version: "v1", Items: []testlab.RubricItem{
		{ID: "c1", Text: "quote the sentence that shows it", Weight: &weight, EvidenceRequired: true},
		{ID: "c2", Text: "an xlsx file is produced", EvidenceRequired: false},
	}}
	results := (&Service{}).merge(m, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "it says so",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{ref}},
			{CriterionID: "c2", Result: ResultPassed, Reason: "the file is there",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: KindArtifact, ArtifactPath: strp("output.xlsx")}}},
		},
	}, digest, false)

	if results[0].Result != ResultUndetermined {
		t.Errorf("an item requiring evidence cannot be passed on a quote nothing verified: %+v", results[0])
	}
	if !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("the downgrade wears the label the UI reads (丙-10): %q", results[0].Reason)
	}
	if len(results[0].Evidence) != 1 {
		t.Error("the citation is still stored; the reader is shown what was offered and why it was not enough")
	}
	// The narrow half of ADR-043 §3: outside `evidence_required` an artifact
	// citation still supports a verdict, because "the file is there" is a fact the
	// platform checked against its own manifest.
	if results[1].Result != ResultPassed {
		t.Errorf("an item that never asked for a quote is not downgraded for lacking one: %+v", results[1])
	}
}

// The floor of §4. This quote's normalised form *is* in the final output; the only
// thing refusing it is its length, which is the whole point — normalisation widens
// matching, and short strings in a widened comparison hit by accident.
func TestAShortQuoteIsNotHandedTheLoosenedComparison(t *testing.T) {
	m, digest := fixtureMaterial(true)
	short := `"Removed 17"` // ten characters once the quote marks are trimmed

	if _, why := verify(llmclient.JudgeEvidenceRef{
		Kind: KindAgentOutput, Quote: short,
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

// --- merge: what actually gets stored -----------------------------------------

func TestUnverifiableEvidenceDowngradesTheVerdictRatherThanBeingStored(t *testing.T) {
	m, digest := fixtureMaterial(true)
	s := &Service{}
	results := s.merge(m, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "trust me",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: KindTraceEvent, TraceEventID: strp("22222222-2222-4222-8222-222222222222")},
				}},
			{CriterionID: "c2", Result: ResultFailed, Reason: "no xlsx",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: KindArtifact, ArtifactPath: strp("output.xlsx")},
				}},
		},
	}, digest, false)

	if len(results) != 2 {
		t.Fatalf("expected one entry per snapshot criterion, got %d", len(results))
	}
	if results[0].Result != ResultUndetermined {
		t.Errorf("a pass resting on an invented event id must not be stored as a pass: %+v", results[0])
	}
	if !strings.HasPrefix(results[0].Reason, "evidence_unverifiable:") {
		t.Errorf("the downgrade has to say why: %q", results[0].Reason)
	}
	if len(results[0].Evidence) != 0 {
		t.Error("a reference that did not resolve must not be stored as evidence")
	}
	if results[1].Result != ResultFailed || len(results[1].Evidence) != 1 {
		t.Errorf("a verifiable failure survives intact: %+v", results[1])
	}
	for _, r := range results {
		if r.Source != SourceModel {
			t.Errorf("everything from the judge is labelled model, got %q", r.Source)
		}
	}
}

// A criterion the model skipped is a criterion with no verdict, not a criterion
// that disappears from the report. The Python side deliberately does not pad the
// list, so this is the Go side's job.
func TestACriterionTheJudgeDidNotAnswerIsUndeterminedAndStillListed(t *testing.T) {
	m, digest := fixtureMaterial(true)
	s := &Service{}
	results := s.merge(m, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "the duplicates are gone",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: KindAgentOutput, Quote: "Removed 17 duplicate rows"},
				}},
			// c2 is missing entirely, and an id nobody asked about is thrown in.
			{CriterionID: "c99", Result: ResultPassed, Reason: "invented"},
		},
	}, digest, false)

	if len(results) != 2 {
		t.Fatalf("the report has one entry per snapshot criterion, got %d", len(results))
	}
	if results[0].Result != ResultPassed {
		t.Errorf("the answered criterion keeps its verdict: %+v", results[0])
	}
	if results[1].CriterionID != "c2" || results[1].Result != ResultUndetermined {
		t.Errorf("the unanswered criterion is listed as undetermined, got %+v", results[1])
	}
	for _, r := range results {
		if r.CriterionID == "c99" {
			t.Error("a criterion id that was never sent must be dropped")
		}
	}
	if overallFrom(results) != OverallPartiallyMet {
		t.Errorf("one pass and one unknown is partially met, got %q", overallFrom(results))
	}
}

func TestVerdictWithoutVerifiedEvidenceIsUndetermined(t *testing.T) {
	m, digest := fixtureMaterial(true)
	results := (&Service{}).merge(m, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "trust me"},
			{CriterionID: "c2", Result: ResultFailed, Reason: "also trust me"},
		},
	}, digest, false)

	for _, result := range results {
		if result.Result != ResultUndetermined {
			t.Errorf("zero-evidence verdict survived: %+v", result)
		}
		if !strings.Contains(result.Reason, "no verifiable evidence") {
			t.Errorf("downgrade does not explain the evidence failure: %q", result.Reason)
		}
	}
}

func TestAPassIsRefusedWhenTheEvidenceCouldBeIncomplete(t *testing.T) {
	pass := llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "looks right",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: KindAgentOutput, Quote: "Removed 17 duplicate rows"}}},
			{CriterionID: "c2", Result: ResultFailed, Reason: "no file",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: KindArtifact, ArtifactPath: strp("output.xlsx")}}},
		},
	}
	s := &Service{}

	incomplete, digest := fixtureMaterial(false)
	got := s.merge(incomplete, pass, digest, false)
	if got[0].Result != ResultUndetermined {
		t.Errorf("a trace with holes cannot support a pass (丙-1), got %q", got[0].Result)
	}
	if got[1].Result != ResultFailed {
		t.Error("a failure is not softened by missing evidence: what is there still contradicts the criterion")
	}

	complete, digest := fixtureMaterial(true)
	got = s.merge(complete, pass, digest, true) // truncated input
	if got[0].Result != ResultUndetermined {
		t.Errorf("judging on truncated input cannot support a pass (§6.3), got %q", got[0].Result)
	}

	got = s.merge(complete, pass, digest, false)
	if got[0].Result != ResultPassed {
		t.Error("with complete, untruncated evidence a pass is a pass")
	}
}

// --- the rubric (CONTENT-007) -------------------------------------------------

// The rubric reaches the judge, and only for criteria that were actually sent.
// An item addressed to something else could never produce a stored verdict —
// merge() drops any id it did not ask about — so sending it would put a standard
// in the prompt that no line of the report is measured against.
func TestTheRubricIsSentOnlyForTheCriteriaTheRequestCarries(t *testing.T) {
	m, _ := fixtureMaterial(true)
	weight := 3.0
	m.rubric = &testlab.Rubric{
		Version: "content-007/writing/v1",
		Items: []testlab.RubricItem{
			{ID: "c1", Text: "quote the sentence that shows it", Weight: &weight, EvidenceRequired: true},
			{ID: "c2", Text: "absence cannot be quoted", EvidenceRequired: false},
			{ID: "gone", Text: "strengthens a criterion this snapshot does not have"},
		},
	}
	s := &Service{}

	req, _, _, dropped := s.buildRequest(m, gen.Evaluation{})
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
	s := &Service{}
	req, _, _, dropped := s.buildRequest(m, gen.Evaluation{})
	if req.Rubric != nil {
		t.Errorf("no rubric means no rubric field, got %+v", req.Rubric)
	}
	if dropped != nil {
		t.Errorf("nothing to drop, got %v", dropped)
	}
	if got := rubricVersion(nil); got != nil {
		t.Errorf("the started event says null, never an empty string, got %#v", got)
	}
	if got := rubricVersion(&testlab.Rubric{Version: "v1"}); got != "v1" {
		t.Errorf("rubricVersion = %#v, want v1", got)
	}
}

// Every item pointing at a criterion the run does not have is the same as having
// no rubric in force, and the version must not be recorded as though one were.
func TestARubricWithNothingLeftToSendIsNotRecordedAsInForce(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.rubric = &testlab.Rubric{
		Version: "content-007/writing/v1",
		Items:   []testlab.RubricItem{{ID: "nobody", Text: "x"}},
	}
	s := &Service{}
	req, _, _, dropped := s.buildRequest(m, gen.Evaluation{})
	if req.Rubric != nil {
		t.Errorf("nothing was left to send, got %+v", req.Rubric)
	}
	if len(dropped) != 1 {
		t.Errorf("the item still has to be reported, got %v", dropped)
	}
}

// --- the digest bounds both the cost and the citation set ---------------------

func TestDigestKeepsTheTailAndSaysWhenItCut(t *testing.T) {
	view := trace.AdvancedView{Complete: true}
	for i := 0; i < maxDigestCount+5; i++ {
		view.Events = append(view.Events, trace.EventView{
			EventID: string(rune('a'+i%26)) + "-" + time.Now().Format("150405") + "-" + strings.Repeat("x", i%3),
			Type:    trace.TypeToolCall,
			Payload: json.RawMessage(`{"n":` + strings.Repeat("1", 1+i%3) + `}`),
		})
	}
	// The last event is the one a verdict most often rests on; it must survive.
	view.Events = append(view.Events, trace.EventView{
		EventID: eventID, Type: trace.TypeAgentOutput, Payload: json.RawMessage(`{"kind":"final"}`),
	})
	// A type nobody may cite.
	view.Events = append(view.Events, trace.EventView{
		EventID: "lifecycle", Type: trace.TypeRunLifecycle, Payload: json.RawMessage(`{}`),
	})

	entries, digest, truncated := buildDigest(view)
	if !truncated {
		t.Error("a digest that dropped events has to report the cut")
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

func TestExcerptsAreCutOnRunesNotBytes(t *testing.T) {
	got, truncated := cut("驗收條件通過了", 3)
	if !truncated || got != "驗收條" {
		t.Errorf("cut on runes: got %q truncated=%v", got, truncated)
	}
	if got, truncated := cut("short", 99); truncated || got != "short" {
		t.Error("nothing under the budget is cut")
	}
}

func TestEvaluationJobsGetOneRecoveryAttempt(t *testing.T) {
	if got := InsertOpts().MaxAttempts; got != 2 {
		t.Fatalf("MaxAttempts = %d, want 2 (one work attempt plus one recovery attempt)", got)
	}
}

func TestDigestReportsAnExcerptCutAsTruncation(t *testing.T) {
	view := trace.AdvancedView{Complete: true, Events: []trace.EventView{{
		EventID: eventID, Type: trace.TypeAgentOutput,
		Payload: json.RawMessage(`{"text":"` + strings.Repeat("x", maxDigestEntry) + `"}`),
	}}}
	entries, _, truncated := buildDigest(view)
	if !truncated {
		t.Fatal("cutting one trace payload was not reported as truncation")
	}
	if len([]rune(entries[0].Excerpt)) != maxDigestEntry {
		t.Fatalf("excerpt length = %d, want %d", len([]rune(entries[0].Excerpt)), maxDigestEntry)
	}
}

// --- the wire call ------------------------------------------------------------

// fakeJudge is apps/llm as llm-internal.yaml describes it, without Python.
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
				{CriterionID: "c1", Result: ResultPassed, Reason: "done", EvidenceRefs: nil},
			},
			Overall: OverallMet, Summary: "the task was completed",
		},
		Model: "gpt-5.6-terra", PromptVersion: "judge-run@2026-08-17",
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
	if resp.Model != "gpt-5.6-terra" || resp.PromptVersion == "" {
		t.Errorf("the caller stores what actually judged, got %+v", resp)
	}
	// The coordinator's point ①: Python reads a missing final_output / entries
	// leniently, so the caller always sends them rather than relying on a 422.
	if got.FinalOutput == "" || got.TraceDigest.Entries == nil || got.Truncation == nil {
		t.Errorf("every field the contract lists is sent, got %+v", got)
	}
}

// A gateway failure, and a value the service refuses outright, both arrive here as
// errors - and an error is an evaluation failure, never a guessed pass.
func TestAJudgeFailureIsAnErrorAndNotALenientVerdict(t *testing.T) {
	client := fakeJudge(t, http.StatusBadGateway, map[string]string{
		"error": "the model returned a result outside the value domain",
	}, nil)
	if _, err := client.JudgeRun(context.Background(), llmclient.JudgeRunRequest{}); err == nil {
		t.Fatal("a 502 from the judge must surface as an error")
	}

	svc := &Service{Judge: client}
	if _, err := svc.judge(context.Background(), material{
		criteria: []testlab.Criterion{{ID: "c1", Text: "x"}},
	}, gen.Evaluation{}); err == nil {
		t.Fatal("judge() propagates the failure so Evaluate can record status=failed")
	}
}

func TestNoJudgeConfiguredIsAFailureAndNotASilentPass(t *testing.T) {
	svc := &Service{}
	if _, err := svc.judge(context.Background(), material{}, gen.Evaluation{}); err == nil {
		t.Fatal("a deployment with no judge cannot produce a task verdict, and must say so")
	}
}

// The internal call carries the caller's cancellation (iron rule 7).
func TestJudgeCallHonoursTheCallersCancellation(t *testing.T) {
	// The handler waits for the test to release it as well as for its own context:
	// a client that goes away does not always make the server notice, and a
	// handler nobody releases would hang Close and not prove anything.
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

// --- the deterministic leg ----------------------------------------------------

func TestActivationNeverClaimsTheModelChoseNotToUseTheSkill(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.run = RunFacts{Status: "succeeded"}
	findings := activationFindings(m)
	if len(findings) != 1 || findings[0].Category != CategoryActivation {
		t.Fatalf("one activation finding expected, got %+v", findings)
	}
	// 丙-4: "available but not used" is not observable in the SDK message stream,
	// so no wording here may imply it.
	for _, forbidden := range []string{"chose", "declined", "ignored", "decided not"} {
		if strings.Contains(strings.ToLower(findings[0].Message), forbidden) {
			t.Errorf("the message claims something the trace cannot show (%q): %s",
				forbidden, findings[0].Message)
		}
	}
}

func TestASuccessfulRunWithNoOutputFilesIsReported(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.run = RunFacts{Status: "succeeded"}
	m.artifacts = nil
	findings := artifactFindings(m)
	if len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Fatalf("handoff 丙-5's case has to be visible, got %+v", findings)
	}

	// A run that never finished has an obvious reason to have written nothing.
	m.run.Status = "failed"
	if got := artifactFindings(m); len(got) != 0 {
		t.Errorf("no finding for a run that did not finish, got %+v", got)
	}
}

func TestCostIsReportedAsALowerBoundAndUnreportedIsNotZero(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.summary.Usage = nil
	if got := costFindings(m); !strings.Contains(got[0].Message, "rather than zero") {
		t.Errorf("an absent usage event is unknown, not $0: %s", got[0].Message)
	}
	cost := 0.0142
	m.summary.Usage = &trace.UsageSummary{
		Model: "gpt-5.4-mini", InputTokens: 100, OutputTokens: 20,
		CostUSD: &cost, CostSource: "gateway",
	}
	if got := costFindings(m); !strings.Contains(got[0].Message, "lower bound") {
		t.Errorf("handoff 丙-3 requires the total to be labelled a lower bound: %s", got[0].Message)
	}
}
