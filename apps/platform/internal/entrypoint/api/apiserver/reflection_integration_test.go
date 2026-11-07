package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/llmadapter"
)

const heldOutFinalOutput = "HELD-OUT-ANSWER the rows were left exactly as they came in."

type reflectionModel struct {
	mu      sync.Mutex
	digests map[string][]string
	focuses []string
	failing map[string]bool
	silent  map[string]bool
	costUSD *float64
	byFocus bool
}

func (m *reflectionModel) record(attribution, digest, focus string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.digests[attribution] = append(m.digests[attribution], digest)
	if strings.HasPrefix(attribution, "batch:") {
		m.focuses = append(m.focuses, focus)
	}
}

func (m *reflectionModel) answerTo(focus string) (fails, silent, byFocus bool, costUSD *float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failing[focus], m.silent[focus], m.byFocus, m.costUSD
}

func (m *reflectionModel) answerEachFocusDifferently() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byFocus = true
}

func proposalsFor(focus string, proposals []llmclient.ImprovementProposal) []llmclient.ImprovementProposal {
	out := slices.Clone(proposals)
	for i := range out {
		out[i].ProposedContent += "\nFocus on " + focus + ".\n"
	}
	return out
}

func (m *reflectionModel) answerFocuses(failing, silent map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failing, m.silent = failing, silent
}

func (m *reflectionModel) sentFocuses() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.focuses...)
}

func (m *reflectionModel) batchDigests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for attribution, digests := range m.digests {
		if strings.HasPrefix(attribution, "batch:") {
			out = append(out, digests...)
		}
	}
	return out
}

func reflectionLLM(t *testing.T, proposals []llmclient.ImprovementProposal) (*llmclient.Client, *reflectionModel) {
	t.Helper()
	model := &reflectionModel{digests: map[string][]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /judge-run", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: failedBoth, Model: "gpt-6-sol", PromptVersion: "judge-run@test",
		})
	})
	mux.HandleFunc("POST /suggest-improvements", func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.SuggestImprovementsRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, `{"detail":"bad"}`, http.StatusUnprocessableEntity)
			return
		}
		model.record(req.EvaluationID, req.EvaluationDigest, req.Focus)
		fails, silent, byFocus, cost := model.answerTo(req.Focus)
		if fails {
			http.Error(w, `{"detail":"upstream"}`, http.StatusInternalServerError)
			return
		}
		answer := proposals
		if byFocus {
			answer = proposalsFor(req.Focus, proposals)
		}
		if silent {
			answer = nil
		}
		_ = json.NewEncoder(w).Encode(llmclient.SuggestImprovementsResponse{
			Suggestions: answer, Model: "gpt-6-sol", PromptVersion: "suggest-improvements/test",
			Usage: callUsage(cost),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}, model
}

type reflectableBatch struct {
	skillID, versionID, batchID string
	devCase, heldCase           string
	devRun, heldRun             string
	model                       *reflectionModel
}

func testCaseOfRun(t *testing.T, pool *pgxpool.Pool, runID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		SELECT s.test_case_id::text FROM runs r JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
		WHERE r.id = $1`, mustUUID(t, runID)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedReflectableBatch(t *testing.T, a *api, pool *pgxpool.Pool, c *client, name string) reflectableBatch {
	t.Helper()
	ctx := context.Background()
	f := reflectableBatch{skillID: importPackage(t, pool, a.packages, c, name, false)}
	f.versionID, _, _, _ = latestVersionOf(t, pool, f.skillID)
	f.devRun = seedRunForVersion(t, pool, c.workspaceID, f.skillID, f.versionID)
	f.heldRun = seedRunForVersion(t, pool, c.workspaceID, f.skillID, f.versionID)
	seedFinalOutput(t, pool, c.workspaceID, f.devRun, suggestionFinalOutput)
	seedFinalOutput(t, pool, c.workspaceID, f.heldRun, heldOutFinalOutput)
	f.devCase, f.heldCase = testCaseOfRun(t, pool, f.devRun), testCaseOfRun(t, pool, f.heldRun)

	llm, model := reflectionLLM(t, []llmclient.ImprovementProposal{{
		Category: "skill", Problem: "the skill never says how to find duplicates",
		Evidence: `"` + suggestionQuote + `"`, TargetPath: "SKILL.md",
		ProposedContent: packagedSkillMD(name) + "\nCompare every column before calling two rows duplicates.\n",
		ExpectedImpact:  "the next run removes duplicate rows",
	}})
	a.evaluations.Judge = llmadapter.JudgeOrNone(llm)
	for _, run := range []string{f.devRun, f.heldRun} {
		if err := a.evaluations.Evaluate(ctx, mustUUID(t, c.workspaceID), mustUUID(t, run)); err != nil {
			t.Fatalf("evaluate %s: %v", run, err)
		}
	}
	a.evaluations.Suggester = llmadapter.SuggesterOrNone(llm)
	f.model = model

	if err := pool.QueryRow(ctx, `
		WITH b AS (
			INSERT INTO evaluation_batches (workspace_id, skill_id, skill_version_id, trials, status)
			VALUES ($1, $2, $3, 1, 'launched') RETURNING id),
		i AS (
			INSERT INTO evaluation_batch_items (batch_id, workspace_id, test_case_id, position, trial, summary_hash, run_id)
			SELECT b.id, $1, t.test_case_id, t.position, 1, 'sha256:summary', t.run_id
			FROM b, (VALUES ($4::uuid, 0, $5::uuid), ($6, 1, $7)) AS t (test_case_id, position, run_id))
		SELECT id::text FROM b`,
		mustUUID(t, c.workspaceID), mustUUID(t, f.skillID), mustUUID(t, f.versionID),
		mustUUID(t, f.devCase), mustUUID(t, f.devRun), mustUUID(t, f.heldCase), mustUUID(t, f.heldRun),
	).Scan(&f.batchID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f reflectableBatch) path() string {
	return "/skills/" + f.skillID + "/evaluation-batches/" + f.batchID + "/reflections"
}

func devBody(ids ...string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return `{"dev_test_case_ids":[` + strings.Join(quoted, ",") + `]}`
}

type reflectionBody struct {
	ReflectionID   string           `json:"reflection_id"`
	DevTestCaseIDs []string         `json:"dev_test_case_ids"`
	Outcome        string           `json:"outcome"`
	RunsRead       int              `json:"runs_read"`
	RunsNotRead    int              `json:"runs_not_read"`
	Suggestions    []suggestionBody `json:"suggestions"`
	Candidates     []candidateBody  `json:"candidates"`
}

func (c *client) reflect(t *testing.T, f reflectableBatch, body string) (int, reflectionBody, string) {
	t.Helper()
	code, raw := c.doRaw(t, http.MethodPost, f.path(), body)
	var out reflectionBody
	_ = json.Unmarshal([]byte(raw), &out)
	return code, out, raw
}

func (c *client) doRaw(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

func TestAReflectionReadsOnlyTheDevelopmentRunsAndKeepsItsSuggestionsOffTheRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "reflect-dev-only")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-dev-only")

	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated {
		t.Fatalf("reflect: got %d %s, want 201", code, raw)
	}
	assertOneKeptSuggestionFromTheDevRun(t, got, raw)
	assertOnlyTheDevRunReachedTheModel(t, f)

	_, runSuggestions, _ := alice.listSuggestions(t, f.devRun)
	for _, s := range runSuggestions {
		if s.SuggestionID == got.Suggestions[0].SuggestionID {
			t.Errorf("the reflection's suggestion %s is listed under the dev run", s.SuggestionID)
		}
	}
	code, listed := alice.doJSON(t, http.MethodGet, f.path(), "")
	reflections, _ := listed["reflections"].([]any)
	if code != http.StatusOK || len(reflections) != 1 {
		t.Fatalf("list: got %d %v, want the one reflection", code, listed)
	}
	if first, _ := reflections[0].(map[string]any); first["reflection_id"] != got.ReflectionID {
		t.Errorf("listed reflection = %v, want %s", first["reflection_id"], got.ReflectionID)
	}
}

func TestApplyingAReflectionsSuggestionBuildsTheNextVersionAndRecordsItsSource(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "reflect-apply")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-apply")
	_, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if len(got.Suggestions) != 1 {
		t.Fatalf("reflection: %s", raw)
	}
	id := got.Suggestions[0].SuggestionID
	if code, decided := alice.decide(t, id, "accepted"); code != http.StatusOK || decided.Decision != "accepted" {
		t.Fatalf("accept a reflection's suggestion: %d %+v", code, decided)
	}

	apply := func(body map[string]any) (int, applyBody) {
		raw, _ := json.Marshal(body)
		code, text := alice.doRaw(t, http.MethodPost, "/skills/"+f.skillID+"/versions/from-suggestions", string(raw))
		var out applyBody
		_ = json.Unmarshal([]byte(text), &out)
		return code, out
	}
	ids := []string{id}
	_, second, raw := alice.reflect(t, f, devBody(f.devCase))
	if len(second.Suggestions) != 1 {
		t.Fatalf("second reflection: %s", raw)
	}
	assertApplyRefusesAnyOtherOwner(t, apply, got.ReflectionID, second.ReflectionID, ids)

	code, built := apply(map[string]any{"reflection_id": got.ReflectionID, "suggestion_ids": ids})
	if code != http.StatusCreated || built.VersionNumber != 2 || len(built.AppliedSuggestionIDs) != 1 {
		t.Fatalf("apply: got %d %+v, want version 2 with the suggestion", code, built)
	}
	var improvedBy string
	if err := pool.QueryRow(context.Background(), `
		SELECT payload->'improved_by'::text FROM outbox_events
		WHERE event_type = 'skill.version_added' AND payload->>'version_id' = $1`, built.VersionID,
	).Scan(&improvedBy); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"evaluation_id": null, "reflection_id": "%s", "suggestion_ids": ["%s"]}`, got.ReflectionID, id)
	if improvedBy != want {
		t.Errorf("improved_by = %s, want %s", improvedBy, want)
	}
	if err := deliverImprovedVersion(t, a, pool, built.VersionID, false); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if recorded := appliedVersionOnReflection(t, alice, f, got.ReflectionID); recorded != built.VersionID {
		t.Errorf("applied version recorded on the reflection's suggestion = %q, want %s", recorded, built.VersionID)
	}
}

func TestAReflectionMustHoldOutACaseAndNameOnlyThisBatchsCasesOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "reflect-shape")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-shape")
	stranger := "00000000-0000-4000-8000-000000000001"

	for name, body := range map[string]string{
		"no case":            devBody(),
		"every case":         devBody(f.devCase, f.heldCase),
		"a foreign case":     devBody(stranger),
		"one case twice":     devBody(f.devCase, f.devCase),
		"an id that is text": `{"dev_test_case_ids":["nope"]}`,
	} {
		if code, _, raw := alice.reflect(t, f, body); code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s, want 400", name, code, raw)
		}
	}
	if n := len(f.model.batchDigests()); n != 0 {
		t.Errorf("refused requests reached the model %d times", n)
	}
	var rounds int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM batch_reflections WHERE batch_id = $1`, mustUUID(t, f.batchID)).Scan(&rounds); err != nil {
		t.Fatal(err)
	}
	if rounds != 0 {
		t.Errorf("refused requests recorded %d rounds", rounds)
	}

	bob := a.login(t, "reflect-shape-bob")
	if code, _, raw := bob.reflect(t, f, devBody(f.devCase)); code != http.StatusNotFound {
		t.Errorf("a stranger reflecting on alice's batch: got %d %s, want 404", code, raw)
	}
	if code, body := bob.doJSON(t, http.MethodGet, f.path(), ""); code != http.StatusNotFound {
		t.Errorf("a stranger listing alice's reflections: got %d %v, want 404", code, body)
	}
}

func TestAReflectionWaitsForEveryDevelopmentTrialButNotForHeldOutOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "reflect-ready")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-ready")
	ctx := context.Background()
	unlink := func(testCaseID string) {
		if _, err := pool.Exec(ctx, `UPDATE evaluation_batch_items SET run_id = NULL WHERE batch_id = $1 AND test_case_id = $2`,
			mustUUID(t, f.batchID), mustUUID(t, testCaseID)); err != nil {
			t.Fatal(err)
		}
	}

	unlink(f.heldCase)
	if code, _, raw := alice.reflect(t, f, devBody(f.devCase)); code != http.StatusCreated {
		t.Errorf("held-out case not yet run: got %d %s, want 201", code, raw)
	}
	if code, _, raw := alice.reflect(t, f, devBody(f.heldCase)); code != http.StatusConflict {
		t.Errorf("development case not yet run: got %d %s, want 409", code, raw)
	}
	if _, err := pool.Exec(ctx, `UPDATE evaluation_batches SET status = 'launching' WHERE id = $1`, mustUUID(t, f.batchID)); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := alice.reflect(t, f, devBody(f.devCase)); code != http.StatusConflict {
		t.Errorf("a launching batch: got %d %s, want 409", code, raw)
	}
}

func TestAReflectionBelowTheStartThresholdIsRefusedBeforeTheModelIsCalled(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	a.startingCredits = 69
	alice := a.login(t, "reflect-credit")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-credit")

	if code, _, raw := alice.reflect(t, f, devBody(f.devCase)); code != http.StatusUnprocessableEntity {
		t.Fatalf("reflect at 69 credits: got %d %s, want 422", code, raw)
	}
	if n := len(f.model.batchDigests()); n != 0 {
		t.Errorf("the model was called %d times for a refused reflection", n)
	}
	if got := creditBalance(t, alice); got != 69 {
		t.Errorf("balance after a refused reflection = %d, want 69", got)
	}
}

func assertOneKeptSuggestionFromTheDevRun(t *testing.T, got reflectionBody, raw string) {
	t.Helper()
	if got.Outcome != "kept" || got.RunsRead != 1 || got.RunsNotRead != 0 || len(got.Suggestions) != 1 {
		t.Fatalf("reflection = %+v, want kept with one run read and one suggestion", got)
	}
	if ev := got.Suggestions[0].Evidence; len(ev) != 1 || !ev[0].Available || !strings.Contains(ev[0].Excerpt, suggestionQuote) {
		t.Errorf("evidence = %+v, want the dev run's quote, still available", ev)
	}
	if strings.Contains(raw, `"run_id"`) {
		t.Errorf("the response carries the internal run id of its evidence: %s", raw)
	}
}

func assertOnlyTheDevRunReachedTheModel(t *testing.T, f reflectableBatch) {
	t.Helper()
	digests := f.model.batchDigests()
	if len(digests) != 1 {
		t.Fatalf("the batch was sent to the model %d times, want once", len(digests))
	}
	if !strings.Contains(digests[0], suggestionQuote) || strings.Contains(digests[0], "HELD-OUT-ANSWER") {
		t.Errorf("digest = %q, want the dev run and nothing of the held-out one", digests[0])
	}
}

func assertApplyRefusesAnyOtherOwner(
	t *testing.T, apply func(map[string]any) (int, applyBody), reflectionID, otherReflectionID string, ids []string,
) {
	t.Helper()
	for _, body := range []map[string]any{
		{"suggestion_ids": ids},
		{"evaluation_id": reflectionID, "reflection_id": reflectionID, "suggestion_ids": ids},
	} {
		if code, _ := apply(body); code != http.StatusBadRequest {
			t.Errorf("apply %v: got %d, want 400 for not exactly one owner", body, code)
		}
	}
	if code, _ := apply(map[string]any{"evaluation_id": reflectionID, "suggestion_ids": ids}); code != http.StatusNotFound {
		t.Errorf("apply a reflection's suggestion as an evaluation's: got %d, want 404", code)
	}
	if code, _ := apply(map[string]any{"reflection_id": otherReflectionID, "suggestion_ids": ids}); code != http.StatusNotFound {
		t.Errorf("apply one round's suggestion through another round: got %d, want 404", code)
	}
}

func appliedVersionOnReflection(t *testing.T, c *client, f reflectableBatch, reflectionID string) string {
	t.Helper()
	code, listed := c.doJSON(t, http.MethodGet, f.path(), "")
	if code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	var recorded string
	for _, r := range listed["reflections"].([]any) {
		r := r.(map[string]any)
		if r["reflection_id"] != reflectionID {
			continue
		}
		for _, s := range r["suggestions"].([]any) {
			recorded, _ = s.(map[string]any)["applied_skill_version_id"].(string)
		}
	}
	return recorded
}

type candidateBody struct {
	Focus         string   `json:"focus"`
	Outcome       string   `json:"outcome"`
	SuggestionIDs []string `json:"suggestion_ids"`
}

func setReflectionCandidates(t *testing.T, a *api, body string) {
	t.Helper()
	operator := a.login(t, freshName("operator-reflection-candidates"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	if code, answer := operatorCall(t, operator, http.MethodPut, "/admin/settings/reflection-candidates", body); code != http.StatusOK {
		t.Fatalf("set reflection candidates %s: got %d %v, want 200", body, code, answer)
	}
}

func reflectionDebits(t *testing.T, c *client) (n int, promptTokens int64) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*), coalesce(sum(prompt_tokens), 0) FROM cost_events
		WHERE kind = 'batch_reflection' AND user_id = $1`, mustUUID(t, c.userID)).Scan(&n, &promptTokens); err != nil {
		t.Fatal(err)
	}
	return n, promptTokens
}

func TestARoundOfOneCandidateAsksOnceWithNoFocusAndListsNoCandidates(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	alice := a.login(t, "reflect-one-candidate")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-one-candidate")

	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated {
		t.Fatalf("reflect: got %d %s, want 201", code, raw)
	}
	if got.Candidates == nil || len(got.Candidates) != 0 {
		t.Errorf("candidates = %#v, want an empty list", got.Candidates)
	}
	if sent := f.model.sentFocuses(); len(sent) != 1 || sent[0] != "" {
		t.Errorf("focuses sent = %q, want one call with no focus", sent)
	}
	if n, tokens := reflectionDebits(t, alice); n != 1 || tokens != 20 {
		t.Errorf("debits = %d for %d prompt tokens, want 1 for 20", n, tokens)
	}
}

func TestARoundOfThreeCandidatesAsksEachFocusOnceAndIsChargedOnce(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":3,"round_credit_cap":2147483647,"note":"compare three"}`)
	alice := a.login(t, "reflect-three-candidates")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-three-candidates")

	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated {
		t.Fatalf("reflect: got %d %s, want 201", code, raw)
	}
	sent := f.model.sentFocuses()
	sort.Strings(sent)
	if want := []string{"instructions", "references_and_examples", "scripts_and_tools"}; !slices.Equal(sent, want) {
		t.Errorf("focuses sent = %q, want %q", sent, want)
	}
	if got.Outcome != "kept" || len(got.Suggestions) != 3 || len(got.Candidates) != 3 {
		t.Fatalf("round = %s, want kept with three candidates of one suggestion each", raw)
	}
	seen := map[string]bool{}
	for i, want := range []string{"instructions", "scripts_and_tools", "references_and_examples"} {
		c := got.Candidates[i]
		if c.Focus != want || c.Outcome != "kept" || len(c.SuggestionIDs) != 1 || seen[c.SuggestionIDs[0]] {
			t.Errorf("candidate %d = %+v, want focus %s kept with its own suggestion", i, c, want)
		}
		if len(c.SuggestionIDs) == 1 {
			seen[c.SuggestionIDs[0]] = true
		}
	}
	if n, tokens := reflectionDebits(t, alice); n != 1 || tokens != 60 {
		t.Errorf("debits = %d for %d prompt tokens, want one debit for all three calls' 60", n, tokens)
	}
}

func TestARoundOfSeveralCandidatesIsRefusedBeforeAnyCallWhileTheRoundCapIsUnset(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":2,"note":"no cap yet"}`)
	alice := a.login(t, "reflect-cap-unset")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-cap-unset")

	code, _, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusConflict || !strings.Contains(raw, "reflection_round_credit_cap") {
		t.Fatalf("reflect: got %d %s, want 409 naming reflection_round_credit_cap", code, raw)
	}
	if n := len(f.model.batchDigests()); n != 0 {
		t.Errorf("the model was called %d times for a refused round", n)
	}
	if n, _ := reflectionDebits(t, alice); n != 0 {
		t.Errorf("a refused round left %d debits", n)
	}
}

func TestARoundWhoseEstimateExceedsTheRoundCapIsRefusedBeforeAnyCall(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":2,"round_credit_cap":1,"note":"tiny cap"}`)
	alice := a.login(t, "reflect-over-cap")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-over-cap")

	if code, _, raw := alice.reflect(t, f, devBody(f.devCase)); code != http.StatusUnprocessableEntity {
		t.Fatalf("reflect: got %d %s, want 422", code, raw)
	}
	if n := len(f.model.batchDigests()); n != 0 {
		t.Errorf("the model was called %d times for a round over its cap", n)
	}
}

func TestEachCandidateOfARoundRecordsItsOwnOutcomeAndTheRoundTakesTheBest(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":3,"round_credit_cap":2147483647,"note":"mixed answers"}`)
	alice := a.login(t, "reflect-mixed-candidates")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-mixed-candidates")
	f.model.answerFocuses(map[string]bool{"scripts_and_tools": true}, map[string]bool{"references_and_examples": true})

	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated {
		t.Fatalf("reflect: got %d %s, want 201", code, raw)
	}
	outcomes := make([]string, len(got.Candidates))
	for i, c := range got.Candidates {
		outcomes[i] = c.Outcome
	}
	if want := []string{"kept", "unavailable", "none_proposed"}; got.Outcome != "kept" || !slices.Equal(outcomes, want) || len(got.Suggestions) != 1 {
		t.Errorf("round %s with candidates %q and %d suggestions, want kept, %q and 1", got.Outcome, outcomes, len(got.Suggestions), want)
	}
	if n, tokens := reflectionDebits(t, alice); n != 1 || tokens != 40 {
		t.Errorf("debits = %d for %d prompt tokens, want one for the two answered calls' 40", n, tokens)
	}
}

func TestARoundWhoseEveryCandidateFailedIsUnavailableAndUncharged(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":2,"round_credit_cap":2147483647,"note":"all fail"}`)
	alice := a.login(t, "reflect-all-fail")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-all-fail")
	f.model.answerFocuses(map[string]bool{"instructions": true, "scripts_and_tools": true}, nil)

	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated || got.Outcome != "unavailable" || len(got.Candidates) != 2 {
		t.Fatalf("reflect: got %d %s, want 201 unavailable with two candidates", code, raw)
	}
	for i, c := range got.Candidates {
		if c.Outcome != "unavailable" || len(c.SuggestionIDs) != 0 || c.SuggestionIDs == nil {
			t.Errorf("candidate %d = %+v, want unavailable with an empty suggestion list", i, c)
		}
	}
	if n, _ := reflectionDebits(t, alice); n != 0 {
		t.Errorf("a round with no answer left %d debits", n)
	}
}

func callUsage(costUSD *float64) *llmclient.GatewayUsage {
	if costUSD == nil {
		return &llmclient.GatewayUsage{PromptTokens: 20, CompletionTokens: 9}
	}
	return &llmclient.GatewayUsage{PromptTokens: 20, CompletionTokens: 9, CostUSD: costUSD, CostSource: llmclient.CostSourceGateway}
}

func (m *reflectionModel) reportCost(usd float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.costUSD = &usd
}

func TestARoundWhoseCallsReportTheirCostListsEachCallOnItsOneDebit(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	setReflectionCandidates(t, a, `{"count":3,"round_credit_cap":2147483647,"note":"priced calls"}`)
	alice := a.login(t, "reflect-priced-calls")
	f := seedReflectableBatch(t, a, pool, alice, "reflect-priced-calls")
	f.model.reportCost(0.002)

	if code, _, raw := alice.reflect(t, f, devBody(f.devCase)); code != http.StatusCreated {
		t.Fatalf("reflect: got %d %s, want 201", code, raw)
	}
	var rows int
	var total int64
	var calls []int64
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) OVER (), usd_micros, call_usd_micros FROM cost_events
		WHERE kind = 'batch_reflection' AND user_id = $1`, mustUUID(t, alice.userID)).Scan(&rows, &total, &calls); err != nil {
		t.Fatal(err)
	}
	if want := []int64{2000, 2000, 2000}; rows != 1 || total != 6000 || !slices.Equal(calls, want) {
		t.Errorf("debits = %d of %d micros listing %v, want one of 6000 listing %v", rows, total, calls, want)
	}
}
