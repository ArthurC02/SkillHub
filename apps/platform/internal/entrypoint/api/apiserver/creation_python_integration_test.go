package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestCreationLangGraphCarriesGoValidationIntoTheNextModelTurn(t *testing.T) {
	python := creationPythonExecutable(t)
	var calls atomic.Int32
	bad := creationDecision("draft", "提出待驗證的草稿。", nil, creationDraft("", "bad"))
	goodDraft := creationDraft("Summarize user input in a requested format.", "good")
	good := creationDecision("draft", "已依 finding 修正描述。", nil, goodDraft)
	review := creationDecision("draft", "已檢查通過的靜態驗證；未宣稱試跑成功。", nil, goodDraft)

	gateway := httptest.NewServer(creationLangGraphGateway(t, &calls, creationTurnScript{bad: bad, good: good, review: review}))
	t.Cleanup(gateway.Close)
	pythonURL := startCreationPython(t, python, gateway.URL)

	a, service, _ := creationFixture(t)

	a.app.CreationSvc.Limits.CallTimeout = 10 * time.Second
	service.Limits.CallTimeout = 10 * time.Second
	service.LLM = creation.ModelOrNone(&llmclient.Client{BaseURL: pythonURL, Token: "test-service"})
	creator := a.login(t, "creation-langgraph-loop")
	v := creationPost(t, creator, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650,
	}, http.StatusOK)

	session := langGraphSession{t: t, creator: creator, service: service, calls: &calls}
	v, badHash := session.draftUntilBlocked(v)
	v, goodHash := session.reviseBlockedDraft(v, badHash)
	session.finalizeReviewedDraft(v, goodHash)
}

type langGraphSession struct {
	t       *testing.T
	creator *client
	service *creation.Service
	calls   *atomic.Int32
}

func (s langGraphSession) draftUntilBlocked(v creation.View) (creation.View, string) {
	t := s.t
	t.Helper()
	v = creationStep(t, s.service, v)
	if v.State != "waiting_confirmation" || v.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("understanding did not request confirmation: %+v", v)
	}
	v = creationAct(t, s.creator, v, "confirm_brief")
	v = creationStep(t, s.service, v)
	if v.State != "queued" || v.Snapshot.Draft == nil || !v.Snapshot.Draft.Blocked || !strings.Contains(v.Snapshot.Draft.Validation, "description-missing") {
		t.Fatalf("bad draft was not retained as a blocked validation result: %+v", v)
	}
	return v, v.Snapshot.Draft.ContentHash
}

func (s langGraphSession) reviseBlockedDraft(v creation.View, badHash string) (creation.View, string) {
	t := s.t
	t.Helper()
	v = creationStep(t, s.service, v)
	if v.State != "queued" || v.Snapshot.Draft == nil || v.Snapshot.Draft.Blocked || v.Snapshot.Draft.ContentHash == badHash {
		t.Fatalf("revised draft was not independently validated: %+v", v)
	}
	goodHash := v.Snapshot.Draft.ContentHash
	var persisted struct {
		PreviousDraft *creation.Draft `json:"previous_draft"`
	}
	var raw []byte
	if err := testPool.QueryRow(context.Background(), "SELECT snapshot FROM creation_sessions WHERE id=$1", v.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.PreviousDraft == nil || persisted.PreviousDraft.ContentHash != badHash || !persisted.PreviousDraft.Blocked {
		t.Fatalf("rejected draft was not kept for the revision trail: %+v", persisted.PreviousDraft)
	}
	return v, goodHash
}

func (s langGraphSession) finalizeReviewedDraft(v creation.View, goodHash string) {
	t := s.t
	t.Helper()
	v = creationStep(t, s.service, v)
	if v.State != "draft_ready" || v.Snapshot.Draft == nil || v.Snapshot.Draft.ContentHash != goodHash || v.Snapshot.Draft.Blocked {
		t.Fatalf("review did not return the exact validated draft: %+v", v)
	}
	v = creationAct(t, s.creator, v, "materialize")
	if v.Snapshot.Candidate == nil {
		t.Fatal("materialize did not create an immutable candidate")
	}
	candidate := *v.Snapshot.Candidate
	v = creationAct(t, s.creator, v, "finalize")
	if v.State != "saved" || v.Snapshot.Candidate == nil || !reflect.DeepEqual(*v.Snapshot.Candidate, candidate) || v.Snapshot.Draft == nil || v.Snapshot.Draft.ContentHash != goodHash || s.calls.Load() != 4 {
		t.Fatalf("finalize changed the candidate or regenerated: %+v calls=%d", v, s.calls.Load())
	}
}

type creationTurnScript struct {
	bad, good, review map[string]any
}

func (s creationTurnScript) respond(call int32, system, prompt string) (map[string]any, string) {
	switch call {
	case 1:
		problem := ""
		if !strings.Contains(system, "Current phase: understand") {
			problem = "first prompt did not use understand phase"
		}
		return creationDecision("confirm_brief", "請確認任務與成功條件。", ptr("整理輸入資料並依指定格式輸出摘要。"), nil), problem
	case 2:
		problem := ""
		if !strings.Contains(system, "Current phase: compose") {
			problem = "second prompt did not use compose phase"
		}
		return s.bad, problem
	case 3:
		problem := ""
		if !strings.Contains(system, "Current phase: revise") || !strings.Contains(prompt, "description-missing") {
			problem = fmt.Sprintf("revision prompt omitted Go finding: %s", prompt)
		}
		return s.good, problem
	case 4:
		problem := ""
		if !strings.Contains(system, "Current phase: review") || !strings.Contains(prompt, `"blocked":false`) {
			problem = fmt.Sprintf("review prompt omitted passing validation: %s", prompt)
		}
		return s.review, problem
	default:
		return s.review, fmt.Sprintf("unexpected model call %d", call)
	}
}

func creationLangGraphGateway(t *testing.T, calls *atomic.Int32, script creationTurnScript) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		system, prompt, ok := readCreationGatewayPrompts(t, w, r)
		if !ok {
			return
		}
		call := calls.Add(1)
		decision, problem := script.respond(call, system, prompt)
		if problem != "" {
			t.Error(problem)
		}
		content, err := json.Marshal(decision)
		if err != nil {
			t.Errorf("marshal decision: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-response-cost", "0.01")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-creation-test", "object": "chat.completion", "created": 1,
			"model": "fixture-model", "choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}
}

func readCreationGatewayPrompts(t *testing.T, w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		t.Errorf("gateway request = %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return "", "", false
	}
	if got := r.Header.Get("Authorization"); got != "Bearer test-attempt-key" {
		t.Errorf("gateway authorization = %q", got)
	}
	var in struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		t.Errorf("decode gateway request: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return "", "", false
	}
	if len(in.Messages) != 2 || in.Messages[1].Role != "user" {
		t.Errorf("gateway messages = %+v", in.Messages)
		http.Error(w, "bad messages", http.StatusBadRequest)
		return "", "", false
	}
	var prompt string
	if err := json.Unmarshal(in.Messages[1].Content, &prompt); err != nil {
		t.Errorf("creation prompt is not text: %v", err)
	}
	var system string
	if err := json.Unmarshal(in.Messages[0].Content, &system); err != nil {
		t.Error(err)
	}
	return system, prompt, true
}

func creationDecision(outcome, message string, brief *string, draft map[string]any) map[string]any {
	return map[string]any{
		"outcome": outcome, "message": message, "brief": brief,
		"acceptance_criteria": nil, "sample_input": nil, "diagram_understanding": nil,
		"diagram_description": nil, "diagram_interpretation": nil,
		"tool_intent": nil, "draft": draft,
	}
}

func creationDraft(description, suffix string) map[string]any {
	return map[string]any{
		"name": "creation-summary", "description": description, "compatibility": "Portable Agent Skills package.",
		"allowed_tools": "", "body": "# Task\nRead user input and summarize the important points.\nAsk for the requested output format when it is missing.\n" + suffix,
		"files": []any{},
	}
}

func ptr(s string) *string { return &s }

func creationPythonExecutable(t *testing.T) string {
	t.Helper()
	path := os.Getenv("SKILLHUB_CREATION_PYTHON")
	if path == "" {

		if os.Getenv("SKILLHUB_REQUIRE_CREATION_PYTHON") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_CREATION_PYTHON=1 but SKILLHUB_CREATION_PYTHON is unset; " +
				"this run would have skipped the only Go-to-Python creation test and still reported success")
		}
		t.Skip("SKILLHUB_CREATION_PYTHON is unset; skipping real Python LangGraph integration")
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("SKILLHUB_CREATION_PYTHON must be an absolute path")
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		t.Fatalf("SKILLHUB_CREATION_PYTHON is not an executable file")
	}
	return path
}

func startCreationPython(t *testing.T, python, gatewayURL string) string {
	t.Helper()
	root := creationRepoRoot(t)
	port := freeLoopbackPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, python, "-m", "uvicorn", "skillhub_llm.app:app", "--host", "127.0.0.1", "--port", port, "--log-level", "error", "--no-access-log")
	cmd.Dir = root
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pythonPath := creationPythonPath(root, os.Getenv("PYTHONPATH"))
	cmd.Env = append(os.Environ(),
		"LITELLM_BASE_URL="+gatewayURL+"/v1",
		"LITELLM_API_KEY=test-service-key",
		"LLM_SERVICE_TOKEN=test-service",
		"LANGSMITH_TRACING=false",
		"PYTHONPATH="+pythonPath,
	)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start Python creation service: %v", err)
	}
	proc := &creationPythonProcess{done: make(chan struct{})}
	go func() { proc.exitErr = cmd.Wait(); close(proc.done) }()
	t.Cleanup(func() {
		cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-proc.done:
		case <-time.After(5 * time.Second):
			t.Error("Python creation service did not exit after kill")
		}
		if t.Failed() && stderr.Len() > 0 {
			t.Logf("Python creation service stderr:\n%s", stderr.String())
		}
	})
	return awaitCreationPythonHealthy(t, "http://127.0.0.1:"+port, proc)
}

type creationPythonProcess struct {
	done    chan struct{}
	exitErr error
}

func freeLoopbackPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func creationPythonPath(root, inherited string) string {
	pythonPath := filepath.Join(root, "apps", "llm", "src") + string(os.PathListSeparator) + filepath.Join(root, "packages", "api-stub-py", "src")
	if inherited != "" {
		pythonPath += string(os.PathListSeparator) + inherited
	}
	return pythonPath
}

func awaitCreationPythonHealthy(t *testing.T, base string, proc *creationPythonProcess) string {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base
			}
		}
		select {
		case <-proc.done:
			t.Fatalf("Python creation service exited before health check: %v", proc.exitErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("Python creation service did not become healthy within 20 seconds")
	return ""
}

func creationRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "apps", "llm", "pyproject.toml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root containing apps/llm/pyproject.toml")
		}
		dir = parent
	}
}
