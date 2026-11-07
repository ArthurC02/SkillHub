package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

type candidateStandingBody struct {
	Candidate      int     `json:"candidate"`
	SkillVersionID *string `json:"skill_version_id"`
	BatchID        *string `json:"batch_id"`
}

type roundBody struct {
	Evaluation *struct {
		Candidates []candidateStandingBody `json:"candidates"`
		Adopted    *struct {
			Candidate      int    `json:"candidate"`
			SkillVersionID string `json:"skill_version_id"`
		} `json:"adopted"`
	} `json:"evaluation"`
	UnconfirmedPairs []struct {
		SkillVersionID string `json:"skill_version_id"`
		TestCaseID     string `json:"test_case_id"`
	} `json:"unconfirmed_pairs"`
}

func (c *client) roundAction(t *testing.T, f reflectableBatch, reflectionID, action, body string) (int, roundBody, string) {
	t.Helper()
	code, raw := c.doRaw(t, http.MethodPost, f.path()+"/"+reflectionID+"/"+action, body)
	var out roundBody
	_ = json.Unmarshal([]byte(raw), &out)
	return code, out, raw
}

func (c *client) confirmEveryPair(t *testing.T, f reflectableBatch, round roundBody) {
	t.Helper()
	if len(round.UnconfirmedPairs) == 0 {
		t.Fatal("no unconfirmed pair to confirm")
	}
	for _, pair := range round.UnconfirmedPairs {
		code, view := c.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/runs/preflight?version_id="+
			url.QueryEscape(pair.SkillVersionID)+"&test_case_id="+url.QueryEscape(pair.TestCaseID), "")
		hash, _ := view["summary_hash"].(string)
		if code != http.StatusOK || hash == "" {
			t.Fatalf("preflight %+v: %d %v", pair, code, view)
		}
		body := fmt.Sprintf(`{"version_id":%q,"test_case_id":%q,"summary_hash":%q}`, pair.SkillVersionID, pair.TestCaseID, hash)
		if code, out := c.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/runs/preflight/confirm", body); code != http.StatusCreated {
			t.Fatalf("confirm %+v: %d %v", pair, code, out)
		}
	}
}

func countEnrichCalls(t *testing.T, a *api) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/enrich-skill" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(llmclient.EnrichSkillResponse{
			Summary: "Removes duplicate rows.", Model: "gpt-6-sol", PromptVersion: "enrich-skill/test",
			Usage: &llmclient.GatewayUsage{PromptTokens: 7, CompletionTokens: 3},
		})
	}))
	t.Cleanup(srv.Close)
	previous := a.versions.LLM
	a.versions.LLM = ingest.ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
	t.Cleanup(func() { a.versions.LLM = previous })
	return &calls
}

func enrichDebits(t *testing.T, c *client) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM cost_events WHERE kind = 'index_enrich' AND workspace_id = $1`,
		mustUUID(t, c.workspaceID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func candidateLine(t *testing.T, pool *pgxpool.Pool, skillID string) (all, held int, newest string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE held_for_evaluation),
		       (SELECT id::text FROM skill_line_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1)
		FROM skill_versions WHERE skill_id = $1`, mustUUID(t, skillID)).Scan(&all, &held, &newest); err != nil {
		t.Fatal(err)
	}
	return all, held, newest
}

func searchIndexedAt(t *testing.T, pool *pgxpool.Pool, skillID string) string {
	t.Helper()
	var at string
	if err := pool.QueryRow(context.Background(),
		`SELECT updated_at::text FROM search_documents WHERE skill_id = $1`, mustUUID(t, skillID)).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func candidateBatches(t *testing.T, pool *pgxpool.Pool, reflectionID string) (made, linked int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*), count(l.batch_id) FROM evaluation_batches b
		LEFT JOIN reflection_candidate_batches l ON l.batch_id = b.id
		WHERE b.origin_key LIKE 'batch-reflection/' || $1 || '/candidate/%'`, reflectionID).Scan(&made, &linked); err != nil {
		t.Fatal(err)
	}
	return made, linked
}

func reflectOnCandidates(t *testing.T, a *api, pool *pgxpool.Pool, name string, candidates int) (*client, reflectableBatch, string) {
	t.Helper()
	clearEvaluationSettings(t, pool)
	setReflectionCandidates(t, a, fmt.Sprintf(`{"count":%d,"round_credit_cap":2147483647,"note":"evaluate candidates"}`, candidates))
	alice := a.login(t, name)
	f := seedReflectableBatch(t, a, pool, alice, name)
	f.model.answerEachFocusDifferently()
	code, got, raw := alice.reflect(t, f, devBody(f.devCase))
	if code != http.StatusCreated || len(got.Suggestions) != candidates {
		t.Fatalf("reflect: got %d %s, want %d suggestions", code, raw, candidates)
	}
	return alice, f, got.ReflectionID
}

func TestEvaluatingCandidatesHoldsOneVersionEachAndAdoptingPromotesOnlyThatOne(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "evaluate-candidates", 2)
	enriched := countEnrichCalls(t, a)
	indexed := searchIndexedAt(t, pool, f.skillID)

	code, refused, raw := alice.roundAction(t, f, round, "evaluate", "")
	if code != http.StatusUnprocessableEntity || len(refused.UnconfirmedPairs) != 4 {
		t.Fatalf("first evaluate: got %d %s, want 422 listing two versions on two cases", code, raw)
	}
	if code, again, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusUnprocessableEntity || len(again.UnconfirmedPairs) != 4 {
		t.Fatalf("evaluate again before confirming: got %d %s, want the same 422", code, raw)
	}
	if all, held, newest := candidateLine(t, pool, f.skillID); all != 3 || held != 2 || newest != f.versionID {
		t.Fatalf("after two refused evaluations: %d versions, %d held, newest %s; want 3, 2 held, newest still %s", all, held, newest, f.versionID)
	}

	evaluated := assertEvaluatingLaunchesOneBatchEach(t, pool, alice, f, round, refused)
	if n, d := enriched.Load(), enrichDebits(t, alice); n != 0 || d != 0 {
		t.Errorf("holding two candidates made %d enrichment calls and %d debits, want none", n, d)
	}
	if at := searchIndexedAt(t, pool, f.skillID); at != indexed {
		t.Errorf("holding two candidates re-indexed the skill at %s, want its index left at %s", at, indexed)
	}
	chosen := *evaluated.Evaluation.Candidates[0].SkillVersionID
	assertAdoptingPromotesOnlyThatCandidate(t, pool, alice, f, round, chosen)
	if n, d := enriched.Load(), enrichDebits(t, alice); n != 1 || d != 1 {
		t.Errorf("adopting twice made %d enrichment calls and %d debits, want exactly one", n, d)
	}
	if at := searchIndexedAt(t, pool, f.skillID); at == indexed {
		t.Errorf("adopting left the skill's index at %s, want it re-indexed", at)
	}
	other := *evaluated.Evaluation.Candidates[1].SkillVersionID
	assertListedStandings(t, alice, f.skillID, map[string][2]bool{
		f.versionID: {false, false}, chosen: {true, true}, other: {true, false},
	})
	assertCandidateVersions(t, alice, f.skillID, []string{"1/0/" + chosen, "1/1/" + other})
	assertCandidateVersions(t, a.login(t, "evaluate-candidates-bob"), f.skillID, nil)
}

func assertEvaluatingLaunchesOneBatchEach(
	t *testing.T, pool *pgxpool.Pool, alice *client, f reflectableBatch, round string, refused roundBody,
) roundBody {
	t.Helper()
	alice.confirmEveryPair(t, f, refused)
	code, evaluated, raw := alice.roundAction(t, f, round, "evaluate", "")
	if code != http.StatusOK || evaluated.Evaluation == nil || len(evaluated.Evaluation.Candidates) != 2 {
		t.Fatalf("evaluate: got %d %s, want 200 with two candidates", code, raw)
	}
	if code, _, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusOK {
		t.Fatalf("evaluate once more: got %d %s, want 200", code, raw)
	}
	if made, linked := candidateBatches(t, pool, round); made != 2 || linked != 2 {
		t.Errorf("candidate batches = %d made, %d linked, want 2 and 2", made, linked)
	}
	if all, _, _ := candidateLine(t, pool, f.skillID); all != 3 {
		t.Errorf("versions after evaluating twice = %d, want 3", all)
	}
	if n := len(f.model.batchDigests()); n != 2 {
		t.Errorf("suggest calls = %d, want the round's 2 and none from evaluating", n)
	}
	var decidedByAlice int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM evaluation_suggestions
		WHERE reflection_id = $1 AND decision = 'accepted' AND decided_by = $2 AND decided_at IS NOT NULL`,
		mustUUID(t, round), mustUUID(t, alice.userID)).Scan(&decidedByAlice); err != nil {
		t.Fatal(err)
	}
	if decidedByAlice != 2 {
		t.Errorf("suggestions accepted by alice's click = %d, want 2", decidedByAlice)
	}
	return evaluated
}

func assertAdoptingPromotesOnlyThatCandidate(t *testing.T, pool *pgxpool.Pool, alice *client, f reflectableBatch, round, chosen string) {
	t.Helper()
	code, adopted, raw := alice.roundAction(t, f, round, "adopt", `{"candidate":0}`)
	if code != http.StatusOK || adopted.Evaluation == nil || adopted.Evaluation.Adopted == nil || adopted.Evaluation.Adopted.SkillVersionID != chosen {
		t.Fatalf("adopt: got %d %s, want candidate 0's version %s adopted", code, raw, chosen)
	}
	if code, _, raw := alice.roundAction(t, f, round, "adopt", `{"candidate":0}`); code != http.StatusOK {
		t.Errorf("adopting the same candidate again: got %d %s, want 200", code, raw)
	}
	if code, _, raw := alice.roundAction(t, f, round, "adopt", `{"candidate":1}`); code != http.StatusConflict {
		t.Errorf("adopting another candidate: got %d %s, want 409", code, raw)
	}
	if _, held, newest := candidateLine(t, pool, f.skillID); newest != chosen || held != 2 {
		t.Errorf("after adopting: newest %s with %d held, want %s with both still held", newest, held, chosen)
	}
}

func assertListedStandings(t *testing.T, c *client, skillID string, want map[string][2]bool) {
	t.Helper()
	code, raw := c.doRaw(t, http.MethodGet, "/skills/"+skillID+"/versions", "")
	var body struct {
		Versions []struct {
			VersionID string `json:"version_id"`
			Held      bool   `json:"held_for_evaluation"`
			Promoted  bool   `json:"promoted"`
		} `json:"versions"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil || code != http.StatusOK || len(body.Versions) != len(want) {
		t.Fatalf("versions: %d %s, want %d versions", code, raw, len(want))
	}
	for _, v := range body.Versions {
		if got := [2]bool{v.Held, v.Promoted}; got != want[v.VersionID] {
			t.Errorf("version %s listed held, promoted = %v, want %v", v.VersionID, got, want[v.VersionID])
		}
	}
}

func assertCandidateVersions(t *testing.T, c *client, skillID string, want []string) {
	t.Helper()
	code, raw := c.doRaw(t, http.MethodGet, "/skills/"+skillID+"/candidate-versions", "")
	var body struct {
		CandidateVersions []struct {
			VersionID string `json:"version_id"`
			Round     int    `json:"round"`
			Candidate int    `json:"candidate"`
		} `json:"candidate_versions"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil || code != http.StatusOK || body.CandidateVersions == nil {
		t.Fatalf("candidate versions: %d %s", code, raw)
	}
	got := make([]string, len(body.CandidateVersions))
	for i, v := range body.CandidateVersions {
		got[i] = fmt.Sprintf("%d/%d/%s", v.Round, v.Candidate, v.VersionID)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidate versions = %v, want %v", got, want)
	}
}

func TestAHeldCandidateThatWasNeverAdoptedCannotStartARound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "evaluate-unadopted", 1)
	_, refused, _ := alice.roundAction(t, f, round, "evaluate", "")
	alice.confirmEveryPair(t, f, refused)
	code, evaluated, raw := alice.roundAction(t, f, round, "evaluate", "")
	if code != http.StatusOK || evaluated.Evaluation == nil || evaluated.Evaluation.Candidates[0].BatchID == nil {
		t.Fatalf("evaluate: got %d %s", code, raw)
	}
	candidateRound := f
	candidateRound.batchID = *evaluated.Evaluation.Candidates[0].BatchID
	code, _, raw = alice.reflect(t, candidateRound, devBody(f.devCase))
	if code != http.StatusConflict || !strings.Contains(raw, "已經不是這個小工具正式路線上的最新版") {
		t.Errorf("reflecting on the unadopted candidate's batch: got %d %s, want 409 saying it is not the line's newest", code, raw)
	}
	registerPublisher(t, alice, freshName("evaluate-unadopted"))
	held := *evaluated.Evaluation.Candidates[0].SkillVersionID
	if code, body := publish(t, alice, f.skillID, `{"rights_attested":true,"version_id":"`+held+`"}`); code != http.StatusUnprocessableEntity ||
		body["reason"] != "held_for_evaluation" {
		t.Errorf("publishing the unadopted candidate: %d %v, want 422 held_for_evaluation", code, body)
	}
}

func TestARejectedSuggestionIsLeftOutSoACandidateOfOnlyRejectedOnesIsUnavailable(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "evaluate-rejected", 2)
	var rejected string
	if err := pool.QueryRow(context.Background(),
		`SELECT id::text FROM evaluation_suggestions WHERE reflection_id = $1 AND candidate = 1`,
		mustUUID(t, round)).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if code, decided := alice.decide(t, rejected, "rejected"); code != http.StatusOK || decided.Decision != "rejected" {
		t.Fatalf("reject candidate 1's suggestion: %d %+v", code, decided)
	}
	_, refused, _ := alice.roundAction(t, f, round, "evaluate", "")
	alice.confirmEveryPair(t, f, refused)
	code, evaluated, raw := alice.roundAction(t, f, round, "evaluate", "")
	if code != http.StatusOK || evaluated.Evaluation == nil || len(evaluated.Evaluation.Candidates) != 2 {
		t.Fatalf("evaluate: got %d %s, want 200 with two candidates", code, raw)
	}
	if built, unavailable := evaluated.Evaluation.Candidates[0], evaluated.Evaluation.Candidates[1]; built.SkillVersionID == nil ||
		unavailable.SkillVersionID != nil || unavailable.BatchID != nil {
		t.Errorf("candidates = %s, want candidate 0 built and candidate 1 unavailable", raw)
	}
	var decision, skipped string
	if err := pool.QueryRow(context.Background(), `
		SELECT s.decision, array_to_string(l.skipped_suggestion_ids, ',')
		FROM evaluation_suggestions s JOIN reflection_candidate_batches l ON l.reflection_id = s.reflection_id AND l.candidate = 1
		WHERE s.id = $1`, mustUUID(t, rejected)).Scan(&decision, &skipped); err != nil {
		t.Fatal(err)
	}
	if decision != "rejected" || skipped != rejected {
		t.Errorf("rejected suggestion = %s, skipped %q; want still rejected and skipped as %s", decision, skipped, rejected)
	}
	if all, held, _ := candidateLine(t, pool, f.skillID); all != 2 || held != 1 {
		t.Errorf("versions = %d with %d held, want 2 with only candidate 0's held", all, held)
	}
}

type failingPuts struct {
	ingest.ObjectStore
	mu      sync.Mutex
	puts    int
	failure int
}

func (s *failingPuts) Put(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	s.puts++
	fail := s.puts == s.failure
	s.mu.Unlock()
	if fail {
		return errors.New("object store went away")
	}
	return s.ObjectStore.Put(ctx, key, data)
}

func TestACrashAfterTwoOfFourCandidatesIsHealedByRetryingWithOneVersionEach(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "evaluate-crash-versions", 4)
	store := a.versions.Store
	a.versions.Store = &failingPuts{ObjectStore: store, failure: 3}
	t.Cleanup(func() { a.versions.Store = store })

	if code, _, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusInternalServerError {
		t.Fatalf("evaluate with the third save failing: got %d %s, want 500", code, raw)
	}
	if _, held, _ := candidateLine(t, pool, f.skillID); held != 2 {
		t.Fatalf("held versions after the crash = %d, want the two built before it", held)
	}
	a.versions.Store = store
	if code, _, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusUnprocessableEntity {
		t.Fatalf("retry: got %d %s, want 422 for the unconfirmed pairs", code, raw)
	}
	if all, held, newest := candidateLine(t, pool, f.skillID); all != 5 || held != 4 || newest != f.versionID {
		t.Errorf("after the retry: %d versions, %d held, newest %s; want 5, 4 held, newest %s", all, held, newest, f.versionID)
	}
}

func refuseInserts(t *testing.T, pool *pgxpool.Pool, table, reflectionID string) (lift func()) {
	t.Helper()
	ctx := context.Background()
	fn := "refuse_" + table + "_" + strings.ReplaceAll(reflectionID, "-", "_")
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'write lost'; END $$;
		CREATE TRIGGER %[1]s BEFORE INSERT ON %[2]s
		FOR EACH ROW WHEN (NEW.reflection_id = '%[3]s') EXECUTE FUNCTION %[1]s();`, fn, table, reflectionID)); err != nil {
		t.Fatal(err)
	}
	lift = func() {
		_, _ = pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %[1]s ON %[2]s; DROP FUNCTION IF EXISTS %[1]s()`, fn, table))
	}
	t.Cleanup(lift)
	return lift
}

func TestALostAdoptionRowAfterThePromotionIsHealedByAdoptingAgain(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "adopt-crash", 1)
	_, refused, _ := alice.roundAction(t, f, round, "evaluate", "")
	alice.confirmEveryPair(t, f, refused)
	_, evaluated, raw := alice.roundAction(t, f, round, "evaluate", "")
	if evaluated.Evaluation == nil {
		t.Fatalf("evaluate: %s", raw)
	}
	chosen := *evaluated.Evaluation.Candidates[0].SkillVersionID
	enriched := countEnrichCalls(t, a)
	lift := refuseInserts(t, pool, "reflection_adoptions", round)

	if code, _, raw := alice.roundAction(t, f, round, "adopt", `{"candidate":0}`); code != http.StatusInternalServerError {
		t.Fatalf("adopt with the adoption row failing: got %d %s, want 500", code, raw)
	}
	if _, _, newest := candidateLine(t, pool, f.skillID); newest != chosen {
		t.Fatalf("newest after the lost adoption row = %s, want the promoted %s", newest, chosen)
	}
	lift()
	code, adopted, raw := alice.roundAction(t, f, round, "adopt", `{"candidate":0}`)
	if code != http.StatusOK || adopted.Evaluation == nil || adopted.Evaluation.Adopted == nil || adopted.Evaluation.Adopted.SkillVersionID != chosen {
		t.Fatalf("adopt again: got %d %s, want candidate 0 adopted", code, raw)
	}
	if n := enriched.Load(); n != 1 {
		t.Errorf("enrichment calls across the crash and the retry = %d, want 1", n)
	}
}

func TestACrashBetweenACandidateBatchAndItsLinkIsHealedByRetryingWithNoOrphan(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, f, round := reflectOnCandidates(t, a, pool, "evaluate-crash-link", 2)
	_, refused, _ := alice.roundAction(t, f, round, "evaluate", "")
	alice.confirmEveryPair(t, f, refused)
	dropTrigger := refuseInserts(t, pool, "reflection_candidate_batches", round)

	if code, _, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusInternalServerError {
		t.Fatalf("evaluate with the link write failing: got %d %s, want 500", code, raw)
	}
	if made, linked := candidateBatches(t, pool, round); made != 1 || linked != 0 {
		t.Fatalf("after the crash: %d batches made, %d linked, want 1 unlinked", made, linked)
	}
	dropTrigger()
	if code, _, raw := alice.roundAction(t, f, round, "evaluate", ""); code != http.StatusOK {
		t.Fatalf("retry: got %d %s, want 200", code, raw)
	}
	if made, linked := candidateBatches(t, pool, round); made != 2 || linked != 2 {
		t.Errorf("after the retry: %d batches made, %d linked, want 2 and 2", made, linked)
	}
}
