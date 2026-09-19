package creation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeStepModel struct {
	result *StepResult
	asked  StepRequest
}

func (f *fakeStepModel) CreationStep(_ context.Context, req StepRequest) (*StepResult, error) {
	f.asked = req
	return f.result, nil
}

func stepOverAWire(t *testing.T, capture *llmclient.CreationStepRequest, reply llmclient.CreationStepResponse) StepModel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/creation/step" {
			t.Errorf("adapter called %s, want /v1/creation/step", r.URL.Path)
		}
		if capture != nil {
			if err := json.NewDecoder(r.Body).Decode(capture); err != nil {
				t.Errorf("decoding the request the adapter sent: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func aStepRequest() StepRequest {
	return StepRequest{
		SessionID:            "session-1",
		Revision:             3,
		Messages:             []Message{{Role: "user", Content: "做一個摘要 Skill"}},
		Brief:                "整理輸入資料",
		AcceptanceCriteria:   []string{"摘要含所有重點"},
		SampleInput:          "會議紀錄",
		BriefConfirmed:       true,
		DiagramUnderstanding: "{}",
		DiagramConfirmed:     true,
		Diagram:              &Diagram{MediaType: "image/png", Data: "AQI="},
		References:           []ReferenceSkill{{Name: "prior", SkillMD: "# prior"}},
		Draft:                &GeneratedSkill{Name: "summary", Files: []GeneratedFile{{Path: "a.py", Content: "x"}}},
		DraftValidation:      &DraftValidation{ContentHash: "hash", Blocked: true, Report: "blocked"},
		AllowedTools:         []string{"search_catalog"},
		TimeoutSeconds:       60,
		MaxOutputTokens:      2048,
		GatewayKey:           "a-key",
	}
}

func aStepResult() *StepResult {
	return &StepResult{
		Outcome:            "draft",
		Message:            "草稿已準備好。",
		Brief:              "整理輸入資料",
		AcceptanceCriteria: []string{"摘要含所有重點"},
		SampleInput:        "會議紀錄",
		Reason:             "",
		ToolIntent:         &ToolIntent{Kind: "search_catalog", Query: "摘要", Queries: []string{"摘要", "summary"}},
		Draft:              &GeneratedSkill{Name: "summary", Body: "# task", Files: []GeneratedFile{{Path: "a.py", Content: "x"}}},
		Model:              "a-model", PromptVersion: "creation@1",
	}
}

func TestEveryStepModelAnswersInTheDomainsOwnWords(t *testing.T) {
	want := aStepResult()
	overTheWire := stepOverAWire(t, nil, llmclient.CreationStepResponse{
		Outcome:            "draft",
		Message:            "草稿已準備好。",
		Brief:              "整理輸入資料",
		AcceptanceCriteria: []string{"摘要含所有重點"},
		SampleInput:        "會議紀錄",
		ToolIntent:         &llmclient.CreationToolIntent{Kind: "search_catalog", Query: "摘要", Queries: []string{"摘要", "summary"}},
		Draft: &llmclient.GeneratedSkill{
			Name: "summary", Body: "# task",
			Files: []llmclient.GeneratedFile{{Path: "a.py", Content: "x"}},
		},
		Model: "a-model", PromptVersion: "creation@1",
	})

	for name, model := range map[string]StepModel{
		"fake": &fakeStepModel{result: want}, "http adapter": overTheWire,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := model.CreationStep(context.Background(), aStepRequest())
			if err != nil {
				t.Fatalf("taking a creation step: %v", err)
			}
			if got.Outcome != want.Outcome || got.Message != want.Message || got.Brief != want.Brief ||
				got.Model != want.Model || got.PromptVersion != want.PromptVersion {
				t.Errorf("step result = %+v, want %+v", got, want)
			}
			if got.ToolIntent == nil || got.ToolIntent.Kind != "search_catalog" ||
				len(got.ToolIntent.Queries) != 2 {
				t.Errorf("tool intent = %+v, want the kind and both queries", got.ToolIntent)
			}
			if got.Draft == nil || got.Draft.Name != "summary" || len(got.Draft.Files) != 1 ||
				got.Draft.Files[0].Path != "a.py" {
				t.Errorf("draft = %+v, want the skill and its one file", got.Draft)
			}
		})
	}
}

func TestTheStepAdapterCarriesTheWholeRequestOntoTheWire(t *testing.T) {
	var sent llmclient.CreationStepRequest
	model := stepOverAWire(t, &sent, llmclient.CreationStepResponse{})

	if _, err := model.CreationStep(context.Background(), aStepRequest()); err != nil {
		t.Fatalf("taking a creation step: %v", err)
	}

	if sent.SessionID != "session-1" || sent.Revision != 3 || sent.Brief != "整理輸入資料" ||
		sent.SampleInput != "會議紀錄" || !sent.BriefConfirmed || !sent.DiagramConfirmed {
		t.Errorf("request on the wire = %+v, want the session, revision and what the creator confirmed", sent)
	}
	if len(sent.Messages) != 1 || sent.Messages[0].Role != "user" {
		t.Errorf("messages on the wire = %+v, want the one the creator wrote", sent.Messages)
	}
	if sent.Diagram == nil || sent.Diagram.Data != "AQI=" {
		t.Errorf("diagram on the wire = %+v, want the image the creator attached", sent.Diagram)
	}
	if len(sent.References) != 1 || sent.References[0].SkillMD != "# prior" {
		t.Errorf("references on the wire = %+v, want the reference skill's own text", sent.References)
	}
	if sent.Draft == nil || sent.Draft.Name != "summary" || len(sent.Draft.Files) != 1 {
		t.Errorf("draft on the wire = %+v, want the draft under revision", sent.Draft)
	}
	if sent.DraftValidation == nil || !sent.DraftValidation.Blocked ||
		sent.DraftValidation.ContentHash != "hash" {
		t.Errorf("validation on the wire = %+v, want what the platform found wrong", sent.DraftValidation)
	}
	if len(sent.AllowedTools) != 1 || sent.TimeoutSeconds != 60 || sent.MaxOutputTokens != 2048 {
		t.Errorf("limits on the wire = %+v, want the tools, timeout and token ceiling", sent)
	}
}

func TestTheGatewayKeyNeverReachesTheRequestBody(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = readAllLimited(r)
		if got := r.Header.Get("X-Creation-Gateway-Key"); got != "a-key" {
			t.Errorf("gateway key header = %q, want the key the domain minted", got)
		}
		_ = json.NewEncoder(w).Encode(llmclient.CreationStepResponse{})
	}))
	t.Cleanup(srv.Close)

	model := ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
	if _, err := model.CreationStep(context.Background(), aStepRequest()); err != nil {
		t.Fatalf("taking a creation step: %v", err)
	}
	if strings.Contains(string(body), "a-key") {
		t.Error("the per-attempt gateway key was written into the request body; it belongs in the " +
			"header alone, and a body is what gets logged")
	}
}

func readAllLimited(r *http.Request) ([]byte, error) {
	buf := make([]byte, 64*1024)
	n, err := r.Body.Read(buf)
	return buf[:n], err
}

func TestAnAbsentModelDoesNotReachCreationLookingPresent(t *testing.T) {
	var unconfigured *llmclient.Client

	if model := ModelOrNone(unconfigured); model != nil {
		t.Error("an unconfigured client arrived as a non-nil StepModel; the `LLM == nil` guard on " +
			"stepping now passes and the first attempt panics")
	}
	if ModelOrNone(&llmclient.Client{}) == nil {
		t.Error("a configured client did not reach creation; no session would ever take a step")
	}
}

func TestOnlyAGatewayPricedStepCarriesACostIntoCreation(t *testing.T) {
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
			model := stepOverAWire(t, nil, llmclient.CreationStepResponse{
				Usage: &llmclient.GatewayUsage{PromptTokens: 10, CostUSD: &cost, CostSource: tc.source},
			})
			got, err := model.CreationStep(context.Background(), aStepRequest())
			if err != nil {
				t.Fatalf("taking a creation step: %v", err)
			}
			if got.Usage == nil || got.Usage.PromptTokens != 10 {
				t.Fatalf("usage = %+v, want the tokens the gateway counted", got.Usage)
			}
			if (got.Usage.ReportedCostUSD() != nil) != tc.wantReported {
				t.Errorf("reported cost = %v, want reported=%v", got.Usage.ReportedCostUSD(), tc.wantReported)
			}
		})
	}
}

func TestWhatCreationStoresKeepsItsFieldNames(t *testing.T) {
	snapshot := Snapshot{
		Messages: []Message{{Role: "user", Content: "做一個摘要 Skill"}},
		Draft: &Draft{
			Revision: 2, ContentHash: "hash", Validation: "ok",
			Skill: GeneratedSkill{
				Name: "summary", Description: "d", Compatibility: "c", AllowedTools: "search",
				Body: "# task", Files: []GeneratedFile{{Path: "a.py", Content: "x"}},
			},
		},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshalling the snapshot: %v", err)
	}

	for _, want := range []string{
		`"messages":[{"role":"user","content":"做一個摘要 Skill"}]`,
		`"name":"summary"`, `"compatibility":"c"`, `"allowed_tools":"search"`, `"body":"# task"`,
		`"files":[{"path":"a.py","content":"x"}]`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("stored snapshot is missing %s; rows written before this type moved into the "+
				"domain would stop decoding, and no migration reshapes them\ngot: %s", want, raw)
		}
	}
}
