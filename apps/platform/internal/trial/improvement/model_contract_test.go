package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeModel struct {
	judged    *Judgement
	suggested *Improvements
	askedTo   JudgeRequest
	askedFor  ImprovementRequest
}

func (f *fakeModel) JudgeRun(_ context.Context, req JudgeRequest) (*Judgement, error) {
	f.askedTo = req
	return f.judged, nil
}

func (f *fakeModel) SuggestImprovements(_ context.Context, req ImprovementRequest) (*Improvements, error) {
	f.askedFor = req
	return f.suggested, nil
}

func modelOverAWire(t *testing.T, path string, capture any, reply any) *llmclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("adapter called %s, want %s", r.URL.Path, path)
		}
		if capture != nil {
			if err := json.NewDecoder(r.Body).Decode(capture); err != nil {
				t.Errorf("decoding the request the adapter sent: %v", err)
			}
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

func aJudgeRequest() JudgeRequest {
	return JudgeRequest{
		RunID:        "run-1",
		EvaluationID: "eval-1",
		Skill:        &JudgedSkill{Name: "invoice reader", Summary: "reads invoices"},
		UserPrompt:   "extract the totals",
		Criteria:     []JudgeCriterion{{ID: "c1", Text: "the total is a number"}},
		Rubric: &JudgeRubric{Items: []JudgeRubricItem{
			{ID: "c1", Text: "quote the figure", EvidenceRequired: true},
		}},
		FinalOutput: "total: 42",
		Artifacts:   []JudgeArtifact{{Path: "out.csv", SizeBytes: 12, ContentType: "text/csv"}},
		TraceDigest: TraceDigest{Complete: true, Entries: []TraceDigestEntry{
			{TraceEventID: "ev-1", OccurredAt: "2026-09-20T00:00:00Z", Type: "tool_call", Excerpt: "read"},
		}},
		Truncation: []string{},
	}
}

func aJudgement() *Judgement {
	return &Judgement{
		Criteria: []CriterionVerdict{{
			CriterionID: "c1", Result: ResultPassed, Reason: "the figure is there",
			Citations: []Citation{{Kind: KindAgentOutput, Quote: "total: 42"}},
		}},
		Overall: string(OverallMet), Summary: "the criteria were met",
		Model: "a-model", PromptVersion: "judge@1",
	}
}

func TestEveryJudgeAnswersInTheDomainsOwnWords(t *testing.T) {
	want := aJudgement()
	overTheWire := JudgeOrNone(modelOverAWire(t, "/judge-run", nil, llmclient.JudgeRunResponse{
		Verdict: llmclient.JudgeVerdict{
			CriterionResults: []llmclient.CriterionVerdict{{
				CriterionID: "c1", Result: ResultPassed, Reason: "the figure is there",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: KindAgentOutput, Quote: "total: 42"}},
			}},
			Overall: string(OverallMet), Summary: "the criteria were met",
		},
		Model: "a-model", PromptVersion: "judge@1",
	}))

	for name, judge := range map[string]Judge{"fake": &fakeModel{judged: want}, "http adapter": overTheWire} {
		t.Run(name, func(t *testing.T) {
			got, err := judge.JudgeRun(context.Background(), aJudgeRequest())
			if err != nil {
				t.Fatalf("judging the run: %v", err)
			}
			if got.Overall != want.Overall || got.Summary != want.Summary ||
				got.Model != want.Model || got.PromptVersion != want.PromptVersion {
				t.Errorf("judgement = %+v, want %+v", got, want)
			}
			if len(got.Criteria) != 1 {
				t.Fatalf("criterion verdicts = %d, want 1", len(got.Criteria))
			}
			cv := got.Criteria[0]
			if cv.CriterionID != "c1" || cv.Result != ResultPassed || cv.Reason != "the figure is there" {
				t.Errorf("criterion verdict = %+v, want c1 passed", cv)
			}
			if len(cv.Citations) != 1 || cv.Citations[0].Quote != "total: 42" {
				t.Errorf("citations = %+v, want the one quote the judge offered", cv.Citations)
			}
		})
	}
}

func TestTheJudgeAdapterCarriesTheWholeRequestOntoTheWire(t *testing.T) {
	var sent llmclient.JudgeRunRequest
	judge := JudgeOrNone(modelOverAWire(t, "/judge-run", &sent, llmclient.JudgeRunResponse{}))

	if _, err := judge.JudgeRun(context.Background(), aJudgeRequest()); err != nil {
		t.Fatalf("judging the run: %v", err)
	}

	if sent.RunID != "run-1" || sent.EvaluationID != "eval-1" || sent.UserPrompt != "extract the totals" ||
		sent.FinalOutput != "total: 42" {
		t.Errorf("request on the wire = %+v, want the run, the evaluation, the prompt and the output", sent)
	}
	if sent.Skill == nil || sent.Skill.Name != "invoice reader" {
		t.Errorf("skill on the wire = %+v, want the one the domain named", sent.Skill)
	}
	if len(sent.Criteria) != 1 || sent.Criteria[0].ID != "c1" {
		t.Errorf("criteria on the wire = %+v, want the one criterion", sent.Criteria)
	}
	if sent.Rubric == nil || len(sent.Rubric.Items) != 1 || !sent.Rubric.Items[0].EvidenceRequired {
		t.Errorf("rubric on the wire = %+v, want the item that demands evidence", sent.Rubric)
	}
	if len(sent.Artifacts) != 1 || sent.Artifacts[0].SizeBytes != 12 {
		t.Errorf("artifacts on the wire = %+v, want the one artifact with its size", sent.Artifacts)
	}
	if !sent.TraceDigest.Complete || len(sent.TraceDigest.Entries) != 1 ||
		sent.TraceDigest.Entries[0].TraceEventID != "ev-1" {
		t.Errorf("trace digest on the wire = %+v, want the one entry and its completeness", sent.TraceDigest)
	}
}

func TestEverySuggesterAnswersInTheDomainsOwnWords(t *testing.T) {
	want := &Improvements{
		Proposals: []ImprovementProposal{{
			Category: string(SuggestionSkill), Problem: "the total is never stated",
			Evidence: "total: 42", TargetPath: "SKILL.md",
			ProposedContent: "always state the total", ExpectedImpact: "the criterion passes",
		}},
		Model: "a-model", PromptVersion: "suggest@1",
	}
	overTheWire := SuggesterOrNone(modelOverAWire(t, "/suggest-improvements", nil,
		llmclient.SuggestImprovementsResponse{
			Suggestions: []llmclient.ImprovementProposal{{
				Category: string(SuggestionSkill), Problem: "the total is never stated",
				Evidence: "total: 42", TargetPath: "SKILL.md",
				ProposedContent: "always state the total", ExpectedImpact: "the criterion passes",
			}},
			Model: "a-model", PromptVersion: "suggest@1",
		}))

	for name, suggester := range map[string]Suggester{
		"fake": &fakeModel{suggested: want}, "http adapter": overTheWire,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := suggester.SuggestImprovements(context.Background(), ImprovementRequest{
				EvaluationID: "eval-1", EvaluationDigest: "what went wrong",
				FileTree:    []string{"SKILL.md"},
				TargetFiles: []TargetFile{{Path: "SKILL.md", Content: "# skill"}},
			})
			if err != nil {
				t.Fatalf("suggesting improvements: %v", err)
			}
			if got.Model != want.Model || got.PromptVersion != want.PromptVersion {
				t.Errorf("improvements = %+v, want %+v", got, want)
			}
			if len(got.Proposals) != 1 {
				t.Fatalf("proposals = %d, want 1", len(got.Proposals))
			}
			if got.Proposals[0] != want.Proposals[0] {
				t.Errorf("proposal = %+v, want %+v", got.Proposals[0], want.Proposals[0])
			}
		})
	}
}

func TestTheSuggesterAdapterCarriesTheWholeRequestOntoTheWire(t *testing.T) {
	var sent llmclient.SuggestImprovementsRequest
	suggester := SuggesterOrNone(modelOverAWire(t, "/suggest-improvements", &sent,
		llmclient.SuggestImprovementsResponse{}))

	_, err := suggester.SuggestImprovements(context.Background(), ImprovementRequest{
		EvaluationID: "eval-1", EvaluationDigest: "what went wrong",
		FileTree:    []string{"SKILL.md"},
		TargetFiles: []TargetFile{{Path: "SKILL.md", Content: "# skill"}},
	})
	if err != nil {
		t.Fatalf("suggesting improvements: %v", err)
	}

	if sent.EvaluationID != "eval-1" || sent.EvaluationDigest != "what went wrong" {
		t.Errorf("request on the wire = %+v, want the evaluation and its digest", sent)
	}
	if len(sent.FileTree) != 1 || sent.FileTree[0] != "SKILL.md" {
		t.Errorf("file tree on the wire = %+v, want the one path", sent.FileTree)
	}
	if len(sent.TargetFiles) != 1 || sent.TargetFiles[0].Content != "# skill" {
		t.Errorf("target files on the wire = %+v, want the file and its content", sent.TargetFiles)
	}
}

func TestAnAbsentModelDoesNotReachTheEvaluationLookingPresent(t *testing.T) {
	var unconfigured *llmclient.Client

	if judge := JudgeOrNone(unconfigured); judge != nil {
		t.Error("an unconfigured client arrived as a non-nil Judge; the `Judge == nil` guard on " +
			"judging now passes and the first call panics")
	}
	if suggester := SuggesterOrNone(unconfigured); suggester != nil {
		t.Error("an unconfigured client arrived as a non-nil Suggester; the `Suggester == nil` guard " +
			"on suggesting now passes and the first call panics")
	}
	if JudgeOrNone(&llmclient.Client{}) == nil || SuggesterOrNone(&llmclient.Client{}) == nil {
		t.Error("a configured client did not reach the evaluation; no run would ever be judged")
	}
}

func TestOnlyAGatewayPricedCallCarriesACostIntoTheDomain(t *testing.T) {
	cost := 0.0042
	for _, tc := range []struct {
		name         string
		source       llmclient.CostSource
		wantReported bool
	}{
		{"priced by the gateway", llmclient.CostSourceGateway, true},
		{"priced by something else", "estimated", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			judge := JudgeOrNone(modelOverAWire(t, "/judge-run", nil, llmclient.JudgeRunResponse{
				Usage: &llmclient.GatewayUsage{PromptTokens: 10, CostUSD: &cost, CostSource: tc.source},
			}))
			got, err := judge.JudgeRun(context.Background(), aJudgeRequest())
			if err != nil {
				t.Fatalf("judging the run: %v", err)
			}
			if got.Usage == nil || got.Usage.PromptTokens != 10 {
				t.Fatalf("usage = %+v, want the tokens the gateway counted", got.Usage)
			}
			if got.Usage.CostReported != tc.wantReported {
				t.Errorf("cost reported = %v, want %v", got.Usage.CostReported, tc.wantReported)
			}
			if (got.Usage.ReportedCostUSD() != nil) != tc.wantReported {
				t.Errorf("reported cost = %v, want reported=%v", got.Usage.ReportedCostUSD(), tc.wantReported)
			}
		})
	}
}
