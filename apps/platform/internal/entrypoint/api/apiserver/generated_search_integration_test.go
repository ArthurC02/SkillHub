package apiserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestGeneratedSearchDeclarationSurvivesPythonGoAndImmutableMaterialization(t *testing.T) {
	python := creationPythonExecutable(t)
	for _, mode := range []string{"generate", "creation"} {
		for _, tc := range []struct{ name, compatibility string }{
			{"generic mention", "Linux"},
			{"declared search", "Linux\nskillhub-web-search"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				gateway := httptest.NewServer(generatedSearchGateway(t, tc.compatibility, &calls))
				t.Cleanup(gateway.Close)
				provider := &llmclient.Client{BaseURL: startCreationPython(t, python, gateway.URL), Token: "test-service"}
				draft, provenance := generatedSearchOverPython(t, provider, mode)
				pool := requireDB(t)
				a := newAPI(t, pool)
				owner := a.login(t, "generated-search-owner")
				result, err := a.versions.MaterializeGeneratedCandidate(t.Context(), workspaceOf(t, pool, owner), draft, provenance, nil)
				if err != nil || result.Report.Blocked || calls.Load() != 1 {
					t.Fatalf("materialization = %+v, %v, model calls=%d", result, err, calls.Load())
				}
				if result.Report.Manifest == nil || result.Report.Manifest.Compatibility != tc.compatibility {
					t.Fatalf("package lost or inferred compatibility: %+v", result.Report.Manifest)
				}
				f := fixture{client: owner, skillID: uuidText(result.Skill.ID), versionID: uuidText(result.Version.ID)}
				f.testCaseID = seedTestCase(t, pool, owner.workspaceID, f.skillID)
				assertGeneratedSearchDisclosure(t, pool, a, f, tc.compatibility)
			})
		}
	}
}

func generatedSearchOverPython(t *testing.T, provider *llmclient.Client, mode string) (ingest.GeneratedSkill, ingest.GeneratedCandidateProvenance) {
	t.Helper()
	const task = "Produce a report from the requested facts."
	if mode == "generate" {
		got, err := ingest.ModelOrNone(provider).GenerateSkill(t.Context(), ingest.GenerateRequest{TaskDescription: task, Within: 10 * time.Second})
		if err != nil || got == nil || got.PromptVersion != "generate-skill/v8" {
			t.Fatalf("direct Python generation = %+v, %v", got, err)
		}
		return got.Skill, ingest.GeneratedCandidateProvenance{TaskDescription: task, Model: got.Model, PromptVersion: got.PromptVersion}
	}
	got, err := creation.ModelOrNone(provider).CreationStep(t.Context(), creation.StepRequest{
		SessionID: "search-declaration", Revision: 1, Brief: task, BriefConfirmed: true,
		Messages: []creation.Message{{Role: "user", Content: task}}, GatewayKey: "test-attempt-key", TimeoutSeconds: 10, MaxOutputTokens: 2000,
	})
	if err != nil || got == nil || got.Draft == nil || got.PromptVersion != "creation-step/v49" {
		t.Fatalf("interactive Python generation = %+v, %v", got, err)
	}
	return wiring.GeneratedSkillForIngest(*got.Draft), ingest.GeneratedCandidateProvenance{TaskDescription: task, Model: got.Model, PromptVersion: got.PromptVersion}
}

func generatedSearchGateway(t *testing.T, compatibility string, calls *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages       []struct{ Role, Content string }
			ResponseFormat struct {
				Schema struct{ Name string } `json:"json_schema"`
			} `json:"response_format"`
		}
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected model route: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" {
			t.Errorf("model messages = %+v", request.Messages)
			return
		}
		for _, rule := range []string{"put `skillhub-web-search` on its own complete line", "The declaration is not permission", "Only when a concrete task step"} {
			if !strings.Contains(request.Messages[0].Content, rule) {
				t.Errorf("actual Python prompt omits %q", rule)
			}
		}
		draft := creationDraft("Report supplied or explicitly requested public facts.", "Use cited sources only when search is declared and permitted.")
		draft["compatibility"] = compatibility
		draft["body"] = "# Report\nRead the supplied facts.\nFor a concrete public-fact lookup, use mcp__skillhub__web_search only when declared and permitted. Cite source URLs and report unknowns when evidence is missing.\n"
		var answer any = draft
		key := "test-service-key"
		if request.ResponseFormat.Schema.Name == "creation_decision" {
			key = "test-attempt-key"
			answer = creationDecision("draft", "草稿已準備好。", nil, draft)
		} else if request.ResponseFormat.Schema.Name != "generated_skill" {
			t.Errorf("unexpected model schema: %q", request.ResponseFormat.Schema.Name)
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("wrong gateway credential for %s", request.ResponseFormat.Schema.Name)
		}
		calls.Add(1)
		content, err := json.Marshal(answer)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-response-cost", "0.01")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "search-declaration", "object": "chat.completion", "created": 1, "model": "fixture-model",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}
}

func assertGeneratedSearchDisclosure(t *testing.T, pool *pgxpool.Pool, a *api, f fixture, compatibility string) {
	t.Helper()
	var stored string
	if err := pool.QueryRow(t.Context(), "SELECT COALESCE(manifest->>'compatibility','') FROM skill_versions WHERE id=$1 AND workspace_id=$2", f.versionID, f.workspaceID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != compatibility {
		t.Fatalf("immutable version compatibility = %q, want %q", stored, compatibility)
	}
	fake := providertest.New("generated-search-runtime", "test-token")
	t.Cleanup(fake.Close)
	capability := providertest.DefaultCapability(fake.Name)
	capability.Runtimes[0].WebSearch = "supported"
	fake.Capability = &capability
	a.runs.Providers = run.NewRegistry(fake.Provider())
	code, summary := f.preflight(t)
	if code != http.StatusOK || summary.Blocked != "" {
		t.Fatalf("generated version preflight = %d %+v", code, summary)
	}
	wantsSearch := compatibility == "Linux\nskillhub-web-search"
	if got := strings.Contains(strings.Join(summary.Summary.Tools, "\n"), "公開資料搜尋"); got != wantsSearch {
		t.Fatalf("generated search disclosure = %v, want %v", got, wantsSearch)
	}
}
