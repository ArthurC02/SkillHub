package apiserver_test

import (
	"context"
	"encoding/json"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func creationLimits() creation.Limits {
	return creation.Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 8, MaxToolCalls: 3, CallTimeout: 2 * time.Second, SessionTimeout: time.Minute, Retention: time.Hour, MaxOutputTokens: 1000}
}
func creationFixture(t *testing.T) (*api, *creation.Service, *atomic.Int32) {
	t.Helper()
	return creationFixtureWithLimits(t, creationLimits())
}
func creationFixtureWithLimits(t *testing.T, limits creation.Limits) (*api, *creation.Service, *atomic.Int32) {
	t.Helper()
	pool := requireDB(t)
	count := &atomic.Int32{}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/creation/step" || r.Header.Get("X-Creation-Gateway-Key") != "test-attempt-key" || r.Header.Get("Authorization") != "Bearer test-service" {
			t.Error("wrong provider boundary")
		}
		count.Add(1)
		var in llmclient.CreationStepRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		cost := .01
		out := llmclient.CreationStepResponse{Outcome: "confirm_brief", Message: "請確認任務與成功條件。", Brief: "整理輸入資料，依指定格式輸出摘要。", DiagramUnderstanding: in.DiagramUnderstanding, Model: "fixture-model", PromptVersion: "creation-test/v1", Usage: &llmclient.GatewayUsage{CostUSD: &cost}}
		if in.Diagram != nil {
			out.Outcome = "confirm_diagram"
			out.DiagramUnderstanding = `{"nodes":["開始","整理輸入","輸出摘要"],"conditions":[],"branches":[],"uncertainties":["需確認格式"]}`
			out.Brief = ""
		}
		if in.BriefConfirmed {
			out.Outcome = "draft"
			out.Message = "草稿已準備好。"
			out.Brief = in.Brief
			out.Draft = &llmclient.GeneratedSkill{Name: "creation-summary", Description: "Summarize user input in the requested format.", Body: "# Task\nRead the user input and summarize the important points.\nAsk for the desired output format when missing.\n", Files: []llmclient.GeneratedFile{}}
		}
		// 05 R-46 (b): the brief proposal carries its acceptance criteria; later
		// turns echo the confirmed list back, as the prompt tells the real model to.
		out.AcceptanceCriteria = in.AcceptanceCriteria
		if out.Outcome == "confirm_brief" && len(out.AcceptanceCriteria) == 0 {
			out.AcceptanceCriteria = []string{"輸出摘要含所有輸入重點"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(model.Close)
	set, err := worker.BuildWorkers(pool, worker.Deps{CreationLimits: limits, LLM: &llmclient.Client{BaseURL: model.URL, Token: "test-service"}})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.IssueKey = func(context.Context, string, string, float64, time.Duration) (string, error) {
		return "test-attempt-key", nil
	}
	set.Creation.RevokeKey = func(context.Context, string) error { return nil }
	transient := httptest.NewServer(set.Creation.TransientHandler("test-worker"))
	t.Cleanup(transient.Close)
	packages := packageStore{}
	app, err := apiserver.NewApp(apiserver.Config{Pool: pool, Store: packages, OAuth: &identity.GitHubOAuth{}, DevLogin: true, GenerateExposed: true, CreationExposed: true, CreationLimits: limits, CreationTransient: creation.TransientClient(transient.URL, "test-worker", 5*time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.ResolveReference = app.CreationSvc.ResolveReference
	handler := app.Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &api{Server: server, auth: app.Auth, app: app, packages: packages, handler: handler}, set.Creation, count
}
func creationID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := testPool.QueryRow(context.Background(), "SELECT gen_random_uuid()").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func creationPost(t *testing.T, c *client, path string, body any, want int) creation.View {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Post(c.base+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		var v any
		_ = json.NewDecoder(res.Body).Decode(&v)
		t.Fatalf("POST %s: %d want %d: %v", path, res.StatusCode, want, v)
	}
	var out creation.View
	if want == 200 {
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// creationPostStatus posts and returns the raw status and body, for tests
// asserting on the refusal's wording rather than a decoded creation.View.
func creationPostStatus(t *testing.T, c *client, path string, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Post(c.base+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(out)
}
func creationAct(t *testing.T, c *client, v creation.View, kind string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind, "content_hash": func() string {
		if v.Snapshot.Draft == nil {
			return ""
		}
		return v.Snapshot.Draft.ContentHash
	}()}, 200)
}
func creationStep(t *testing.T, s *creation.Service, v creation.View) creation.View {
	t.Helper()
	var raw []byte
	if err := testPool.QueryRow(context.Background(), "SELECT args FROM river_job WHERE kind='creation_step' AND args->>'session_id'=$1 ORDER BY id DESC LIMIT 1", v.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var a creation.JobArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(context.Background(), a, nil); err != nil {
		t.Fatal(err)
	}
	out, err := s.Get(context.Background(), identity.Workspace{ID: a.WorkspaceID}, a.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestCreationJourneyPreservesConfirmedCandidateAndWorkspace(t *testing.T) {
	a, s, calls := creationFixture(t)
	alice := a.login(t, "creation-alice")
	bob := a.login(t, "creation-bob")
	id := creationID(t)
	input := map[string]any{"id": id, "message": "請建立資料摘要 Skill。", "budget_usd": .5}
	v := creationPost(t, alice, "/creation-sessions", input, 200)
	replay := creationPost(t, alice, "/creation-sessions", input, 200)
	if replay.Revision != v.Revision {
		t.Fatal("create replay advanced")
	}
	if bob.status(t, "GET", "/creation-sessions/"+v.ID) != 404 {
		t.Fatal("cross-workspace session leaked")
	}
	v = creationStep(t, s, v)
	if v.State != "waiting_confirmation" || v.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("missing brief: %+v", v)
	}
	v = creationAct(t, alice, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.State != "draft_ready" || v.Snapshot.Draft == nil || v.Snapshot.Draft.Blocked {
		t.Fatalf("no validated draft: %+v", v)
	}
	draftHash := v.Snapshot.Draft.ContentHash
	v = creationAct(t, alice, v, "materialize")
	if v.Snapshot.Candidate == nil {
		t.Fatal("candidate not committed")
	}
	candidate := *v.Snapshot.Candidate
	v = creationAct(t, alice, v, "finalize")
	if v.State != "saved" || *v.Snapshot.Candidate != candidate || v.Snapshot.Draft.ContentHash != draftHash || calls.Load() != 2 {
		t.Fatalf("finalize regenerated candidate: %+v calls=%d", v, calls.Load())
	}
	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "confirm_brief"}, 422)
	var versions int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM skill_versions WHERE skill_id=$1", candidate.SkillID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 {
		t.Fatalf("finalization created %d versions", versions)
	}
}
func TestCreationDiagramUsesTransientWorkerAndStoresNoImage(t *testing.T) {
	a, _, calls := creationFixture(t)
	c := a.login(t, "creation-diagram")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)
	const image = "cHJpdmF0ZS1mbG93Y2hhcnQtYnl0ZXMtY3JlYXRpb24="
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram", "diagram": map[string]string{"media_type": "image/png", "data": image}}, 200)
	if v.State != "waiting_confirmation" || v.Snapshot.PendingAction != "confirm_diagram" || calls.Load() != 1 {
		t.Fatalf("diagram not processed: %+v", v)
	}
	var found bool
	err := testPool.QueryRow(context.Background(), `SELECT EXISTS(
 SELECT 1 FROM creation_sessions WHERE id=$1 AND snapshot::text LIKE '%'||$2||'%'
 UNION ALL SELECT 1 FROM creation_session_events WHERE session_id=$1 AND snapshot::text LIKE '%'||$2||'%'
 UNION ALL SELECT 1 FROM creation_receipts WHERE session_id=$1 AND (result::text||usage::text) LIKE '%'||$2||'%'
 UNION ALL SELECT 1 FROM river_job WHERE args->>'session_id'=$1::text AND args::text LIKE '%'||$2||'%')`, v.ID, image).Scan(&found)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("original diagram persisted")
	}
}
func TestCreationCommandReplayCASAndQueuedCancellation(t *testing.T) {
	a, s, calls := creationFixture(t)
	c := a.login(t, "creation-cas")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)
	command := map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": "建立摘要"}
	queued := creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", command, 200)
	replay := creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", command, 200)
	if replay.Revision != queued.Revision {
		t.Fatal("replay advanced")
	}
	command["message"] = "changed"
	creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", command, 409)
	creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "cancel"}, 409)
	stopped := creationAct(t, c, queued, "cancel")
	stopped = creationStep(t, s, stopped)
	if stopped.State != "cancelled" || calls.Load() != 0 {
		t.Fatal("cancelled queue called model")
	}
	var pending int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM creation_receipts WHERE session_id=$1 AND status='queued'", v.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatal("cancel left queued receipt")
	}
}
func TestCreationExposureAndAnonymousAuth(t *testing.T) {
	a, _, _ := creationFixture(t)
	for _, path := range []string{"/creation-sessions", "/creation-sessions/" + creation.UUID(creationID(t))} {
		res, err := http.Get(a.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatalf("%s anonymous = %d", path, res.StatusCode)
		}
	}
	hidden := newAPI(t, requireDB(t))
	res, err := http.Get(hidden.URL + "/creation-sessions")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal("disabled creation exposed")
	}
}

type creationStepFunc func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error)

func (f creationStepFunc) CreationStep(ctx context.Context, r llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
	return f(ctx, r)
}
func creationJob(t *testing.T, id string) creation.JobArgs {
	t.Helper()
	var raw []byte
	if err := testPool.QueryRow(context.Background(), "SELECT args FROM river_job WHERE kind='creation_step' AND args->>'session_id'=$1 ORDER BY id DESC LIMIT 1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var a creation.JobArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	return a
}
func TestCreationCancellationSettlesUnknownCostWithoutDraft(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-running-cancel")
	started := make(chan struct{})
	s.LLM = creationStepFunc(func(ctx context.Context, _ llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_usd": .5}, 200)
	job := creationJob(t, v.ID)
	done := make(chan error, 1)
	go func() { done <- s.Step(context.Background(), job, nil) }()
	<-started
	ws := identity.Workspace{ID: job.WorkspaceID}
	working, err := s.Get(context.Background(), ws, job.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	creationAct(t, c, working, "cancel")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	final, err := s.Get(context.Background(), ws, job.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "cancelled" || !final.Snapshot.UsageUnknown || final.Snapshot.ReservedUSD != .1 || final.Snapshot.Draft != nil {
		t.Fatalf("late response changed cancellation or freed unknown spend: %+v", final)
	}
}
func TestCreationPurgeFencesLateModelResult(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-purge-late")
	started := make(chan struct{})
	s.LLM = creationStepFunc(func(ctx context.Context, _ llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_usd": .5}, 200)
	job := creationJob(t, v.ID)
	done := make(chan error, 1)
	go func() { done <- s.Step(context.Background(), job, nil) }()
	<-started
	tx, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PurgeWorkspace(context.Background(), tx, job.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(context.Background(), identity.Workspace{ID: job.WorkspaceID}, job.SessionID); err != creation.ErrNotFound {
		t.Fatalf("purged session returned: %v", err)
	}
	var rows int
	if err = testPool.QueryRow(context.Background(), "SELECT count(*) FROM creation_receipts WHERE session_id=$1", v.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("late completion resurrected receipts")
	}
}
func TestCreationFixedReferencesAreReauthorizedBeforeEachCall(t *testing.T) {
	a, s, calls := creationFixture(t)
	owner := a.login(t, "creation-reference-owner")
	other := a.login(t, "creation-reference-other")
	v := creationPost(t, owner, "/creation-sessions", map[string]any{"id": creationID(t), "message": "建立參考範本", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, owner, v, "confirm_brief")
	v = creationStep(t, s, v)
	v = creationAct(t, owner, v, "finalize")
	candidate := v.Snapshot.Candidate
	newSession := func(c *client) creation.View {
		return creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)
	}
	private := newSession(other)
	creationPost(t, other, "/creation-sessions/"+private.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": private.Revision, "kind": "select_references", "reference_skill_ids": []string{candidate.SkillID}}, 404)
	own := newSession(owner)
	own = creationPost(t, owner, "/creation-sessions/"+own.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": own.Revision, "kind": "select_references", "reference_skill_ids": []string{candidate.SkillID}}, 200)
	if own.Snapshot.References[0].VersionID != candidate.VersionID {
		t.Fatal("reference version not fixed")
	}
	own = creationAct(t, owner, own, "confirm_references")
	if _, err := testPool.Exec(context.Background(), "UPDATE skills SET access_restriction='test hold' WHERE id=$1", candidate.SkillID); err != nil {
		t.Fatal(err)
	}
	own = creationStep(t, s, own)
	if own.State != "waiting_confirmation" || own.Snapshot.References[0].Available || calls.Load() != 2 || own.Snapshot.ReservedUSD != 0 {
		t.Fatalf("unavailable private reference reached model: %+v calls=%d", own, calls.Load())
	}
}

func TestCreationBudgetExhaustionDoesNotEnqueueAnotherCall(t *testing.T) {
	a, s, calls := creationFixture(t)
	c := a.login(t, "creation-budget")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_usd": .1}, 200)
	v = creationStep(t, s, v)
	creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "confirm_brief"}, 422)
	var jobs int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM river_job WHERE kind='creation_step' AND args->>'session_id'=$1", v.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || calls.Load() != 1 || v.Snapshot.SpentUSD == nil || *v.Snapshot.SpentUSD != .01 {
		t.Fatalf("budget boundary failed: jobs=%d calls=%d snapshot=%+v", jobs, calls.Load(), v.Snapshot)
	}
}

func TestCreationRestartRecoversUnknownAttemptWithoutReplay(t *testing.T) {
	a, s, calls := creationFixture(t)
	c := a.login(t, "creation-restart")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_usd": .5}, 200)
	job := creationJob(t, v.ID)
	ctx := context.Background()
	// A killed process leaves only its committed claim and reservation behind.
	if _, err := testPool.Exec(ctx, `UPDATE creation_sessions SET state='working', updated_at=now()-interval '1 minute',
	 snapshot=jsonb_set(jsonb_set(snapshot, '{snapshot,reserved_usd}', '0.1'), '{active_deadline}', to_jsonb(now()-interval '1 minute')) WHERE id=$1`, job.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE creation_receipts SET status='running' WHERE id=$1", job.ReceiptID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.Recover(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Step(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	final, err := s.Get(ctx, identity.Workspace{ID: job.WorkspaceID}, job.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := testPool.QueryRow(ctx, "SELECT status FROM creation_receipts WHERE id=$1", job.ReceiptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if final.State != "failed" || !final.Snapshot.UsageUnknown || final.Snapshot.ReservedUSD != .1 || status != "unknown" || calls.Load() != 0 {
		t.Fatalf("restart replayed or forgot spend: %+v receipt=%s calls=%d", final, status, calls.Load())
	}
}

// The fixture model always names the draft "creation-summary" (see
// creationFixture above), so a second, independent session that reaches
// materialize collides with the skill the first session already created.
// ingest.ErrGeneratedNameCollision must reach the caller as a 422 naming the
// collision, not the handler's 503 default — which a client can never clear
// by retrying, since the workspace still has the same name in it.
func TestCreationSecondSessionWithACollidingNameIsRefusedActionably(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-collision")
	toDraftReady := func() creation.View {
		v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
		v = creationStep(t, s, v)
		v = creationAct(t, c, v, "confirm_brief")
		return creationStep(t, s, v)
	}
	first := toDraftReady()
	first = creationAct(t, c, first, "materialize")
	if first.Snapshot.Candidate == nil {
		t.Fatalf("first materialize did not create a candidate: %+v", first)
	}
	before := countRow(t, testPool, "SELECT count(*) FROM skills WHERE workspace_id=$1", mustUUID(t, c.workspaceID))

	second := toDraftReady()
	status, body := creationPostStatus(t, c, "/creation-sessions/"+second.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": second.Revision, "kind": "materialize",
		"content_hash": second.Snapshot.Draft.ContentHash,
	})
	if status != 422 || !strings.Contains(body, "同名") {
		t.Fatalf("second materialize: got %d %s", status, body)
	}
	after := countRow(t, testPool, "SELECT count(*) FROM skills WHERE workspace_id=$1", mustUUID(t, c.workspaceID))
	if after != before {
		t.Fatalf("collision materialize grew skills: before=%d after=%d", before, after)
	}
}

// creationLimits() bounds Create's budget to [0.1, 1]; a request outside that
// band must be told the band, not handed the same "已達這次核准的創作限制" sentence
// a mid-session ceiling produces.
func TestCreationBudgetOutOfBandNamesTheBand(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-budget-band")
	status, body := creationPostStatus(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "x", "budget_usd": 0.05})
	if status != 422 || !strings.Contains(body, "0.1") || !strings.Contains(body, "1") {
		t.Fatalf("out-of-band budget: got %d %s", status, body)
	}
}

// The published ceilings must match what the fixture actually enforces
// (creationLimits()'s MaxCallCostUSD/MaxCostUSD), so a client can show
// used/allowed instead of guessing a budget.
func TestCreationLimitsEndpoint(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-limits")
	res, err := c.Get(c.base + "/creation-sessions/limits")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("limits: got %d", res.StatusCode)
	}
	var out struct {
		MinBudgetUSD float64 `json:"min_budget_usd"`
		MaxBudgetUSD float64 `json:"max_budget_usd"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.MinBudgetUSD != .1 || out.MaxBudgetUSD != 1 {
		t.Fatalf("limits body: %+v", out)
	}
}

// creationDeadlineLimits mirrors creationLimits() but shortens the session
// wall clock to 2s (Limits.Valid requires SessionTimeout >= CallTimeout,
// which creationLimits() already sets to 2s), so a deadline can be reached in
// a unit test without waiting out the default one-minute ceiling.
func creationDeadlineLimits() creation.Limits {
	l := creationLimits()
	l.SessionTimeout = 2 * time.Second
	return l
}

// A session past its deadline must be told about the deadline, in words
// distinct from the mid-session budget/step/tool-call ceiling sentence
// ("已達這次核准的創作限制") — otherwise a user who ran out of time reads a message
// about running out of money.
func TestCreationDeadlineIsNotTheBudgetSentence(t *testing.T) {
	a, _, _ := creationFixtureWithLimits(t, creationDeadlineLimits())
	c := a.login(t, "creation-deadline")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_usd": .5}, 200)
	time.Sleep(2100 * time.Millisecond)
	status, body := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": "繼續",
	})
	if status != 422 || !strings.Contains(body, "時間上限") || strings.Contains(body, "核准的創作限制") {
		t.Fatalf("deadline: got %d %s", status, body)
	}
}
