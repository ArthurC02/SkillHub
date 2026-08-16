package eval

// Unit coverage for the part of EVAL-001 that decides what gets stored: the
// downgrade rules of ADR-026 defence 3, the digest that bounds both cost and the
// citation set, and the wire call to services/llm.
//
// The judge here is an httptest server speaking the llm-internal.yaml shapes, not
// the real Python service: what is under test is the caller's half - deadline,
// failure handling, and the fact that nothing a model returns is trusted without
// being re-resolved first.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

// --- fixtures ----------------------------------------------------------------

const (
	eventID     = "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01"
	finalOutput = "Removed 17 duplicate rows and saved the result to output.xlsx."
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
		artifacts: []gen.Artifact{
			{FileName: "output.xlsx", SizeBytes: 4096, ContentType: "application/vnd.ms-excel", ContentHash: "sha256:abc"},
		},
	}
	return m, map[string]trace.EventView{eventID: event}
}

func strp(s string) *string { return &s }

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
	quote := "Removed 17 duplicate rows"

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
				if finalOutput[got.CharRange.Start:got.CharRange.End] != quote {
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

func TestAPassIsRefusedWhenTheEvidenceCouldBeIncomplete(t *testing.T) {
	pass := llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: ResultPassed, Reason: "looks right"},
			{CriterionID: "c2", Result: ResultFailed, Reason: "no file"},
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

// --- the wire call ------------------------------------------------------------

// fakeJudge is services/llm as llm-internal.yaml describes it, without Python.
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
	m.run = gen.Run{Status: gen.RunStatusSucceeded}
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
	m.run = gen.Run{Status: gen.RunStatusSucceeded}
	m.artifacts = nil
	findings := artifactFindings(m)
	if len(findings) != 1 || findings[0].Severity != SeverityWarning {
		t.Fatalf("handoff 丙-5's case has to be visible, got %+v", findings)
	}

	// A run that never finished has an obvious reason to have written nothing.
	m.run.Status = gen.RunStatusFailed
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
