package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func betaAPI(
	t *testing.T, pool *pgxpool.Pool,
	quota policy.QuotaLimits, invited []string, retention time.Duration,
) *api {
	t.Helper()
	return newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Runs.Svc.Quota = quota
		if len(invited) > 0 {
			d.Auth.Invited = map[string]bool{}
			for _, id := range invited {
				d.Auth.Invited[id] = true
			}
		}
		d.Analytics.Svc.Retention = retention
	})
}

func seedCountedRun(t *testing.T, pool *pgxpool.Pool, f fixture, failureClass string, at time.Time) {
	t.Helper()

	status, class := "succeeded", any(nil)
	if failureClass != "" {
		status, class = "failed", any(failureClass)
	}
	seedBetaRun(t, pool, f, status, class, &at)
}

func seedBetaRun(t *testing.T, pool *pgxpool.Pool, f fixture, status string, failureClass any, preparedAt *time.Time) string {
	t.Helper()
	ctx := context.Background()
	var snapshotID, runID string
	err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'do the thing', '[]'::jsonb, md5(random()::text))
		RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID)).Scan(&snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, failure_class)
		VALUES ($1, $2, $3, 'seed', '{}'::jsonb, '{}'::jsonb, $4, $5)
		RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, snapshotID), status, failureClass,
	).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	if preparedAt != nil {
		if _, err := pool.Exec(ctx, `
			INSERT INTO run_status_transitions (run_id, workspace_id, from_status, to_status, occurred_at)
			VALUES ($1, $2, 'provisioning', 'preparing', $3)`,
			mustUUID(t, runID), mustUUID(t, f.workspaceID), *preparedAt,
		); err != nil {
			t.Fatal(err)
		}
	}
	return runID
}

type quotaView struct {
	RemainingToday  int    `json:"remaining_today"`
	RemainingWindow int    `json:"remaining_window"`
	WindowResetsAt  string `json:"window_resets_at"`
	Limits          struct {
		Daily      int `json:"daily"`
		Window     int `json:"window"`
		WindowDays int `json:"window_days"`
		Concurrent int `json:"concurrent"`
	} `json:"limits"`
}

func (c *client) quota(t *testing.T) (int, quotaView) {
	t.Helper()
	resp, err := c.Get(c.base + "/me/quota")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out quotaView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) analyticsSession(t *testing.T) string {
	t.Helper()
	for _, cookie := range c.Jar.Cookies(mustURL(t, c.base)) {
		if cookie.Name == "sh_analytics" {
			return cookie.Value
		}
	}
	return ""
}

func betaCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunIsRefusedWhenTheDailyAllowanceIsSpent(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 2, Window: 30, FirstWindow: 30, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-quota-daily")

	for range 2 {
		seedCountedRun(t, pool, f, "", time.Now().Add(-time.Hour))
	}
	before := betaCount(t, pool, `SELECT count(*) FROM runs WHERE workspace_id = $1`, mustUUID(t, f.workspaceID))

	hash := f.confirmPermissions(t)
	code, view := f.startWithHash(t, hash)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST run over the daily allowance: got %d, want 422 (%s)", code, view.Error)
	}
	if !strings.Contains(view.Error, "allowance") {
		t.Errorf("refusal does not say it is the allowance: %q", view.Error)
	}

	if after := betaCount(t, pool, `SELECT count(*) FROM runs WHERE workspace_id = $1`, mustUUID(t, f.workspaceID)); after != before {
		t.Errorf("a refused run left %d rows behind", after-before)
	}
}

func TestFirstWindowUsesTheLowerAllowance(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 50, Window: 30, FirstWindow: 3, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-quota-first-window")

	code, q := f.quota(t)
	if code != http.StatusOK {
		t.Fatalf("GET /me/quota: got %d", code)
	}
	if q.Limits.Window != 3 {
		t.Errorf("first window ceiling: got %d, want the first-window value 3", q.Limits.Window)
	}
	if q.Limits.Concurrent != run.MaxConcurrentRunsPerWorkspace {
		t.Errorf("concurrency limit reported as %d, want %d", q.Limits.Concurrent, run.MaxConcurrentRunsPerWorkspace)
	}

	for range 3 {
		seedCountedRun(t, pool, f, "", time.Now().Add(-2*time.Hour))
	}
	if _, q := f.quota(t); q.RemainingWindow != 0 {
		t.Errorf("after 3 counted runs remaining_window is %d, want 0", q.RemainingWindow)
	}
	hash := f.confirmPermissions(t)
	if code, view := f.startWithHash(t, hash); code != http.StatusUnprocessableEntity {
		t.Fatalf("POST run over the window allowance: got %d, want 422 (%s)", code, view.Error)
	}
}

func TestOnlyPlatformSideFailuresAreRefunded(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 100, Window: 100, FirstWindow: 100, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)

	cases := []struct {
		failureClass string
		counted      bool
	}{
		{"provider_error", false},
		{"platform_error", false},
		{"capability_mismatch", false},
		{"workload_error", true},
		{"cancelled", true},
		{"timeout", true},
		{"", true},
	}
	for _, tc := range cases {
		t.Run("class="+tc.failureClass, func(t *testing.T) {
			f := newFixture(t, a, pool, "quota-refund-"+tc.failureClass)
			_, before := f.quota(t)
			seedCountedRun(t, pool, f, tc.failureClass, time.Now().Add(-time.Hour))
			_, after := f.quota(t)

			spent := before.RemainingWindow - after.RemainingWindow
			want := 0
			if tc.counted {
				want = 1
			}
			if spent != want {
				t.Errorf("failure_class %q spent %d of the allowance, want %d", tc.failureClass, spent, want)
			}
		})
	}
}

func TestRunsThatNeverReachedPreparingDoNotCount(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 10, Window: 10, FirstWindow: 10, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-quota-provisioning")

	f.start(t)

	_, q := f.quota(t)
	if q.RemainingWindow != 10 {
		t.Errorf("a run that never prepared spent %d of the allowance", 10-q.RemainingWindow)
	}
}

func TestTwoSimultaneousRunsCannotBothTakeTheLastSlot(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 50, Window: 50, FirstWindow: 50, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-concurrency-race")

	seedBetaRun(t, pool, f, "running", nil, nil)

	hash := f.confirmPermissions(t)
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		codes []int
	)
	for range 2 {
		wg.Go(func() {
			code, _ := f.startWithHash(t, hash)
			mu.Lock()
			codes = append(codes, code)
			mu.Unlock()
		})
	}
	wg.Wait()

	created, refused := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusUnprocessableEntity:
			refused++
		default:
			t.Fatalf("unexpected status %d from a concurrent create", c)
		}
	}
	if created != 1 || refused != 1 {
		t.Errorf("two simultaneous creates on the last slot produced %d created / %d refused, want 1/1",
			created, refused)
	}
}

func TestQuotaIsNotShownWhereItIsNotEnforced(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "alice-no-quota")

	if code := f.status(t, http.MethodGet, "/me/quota"); code != http.StatusNotFound {
		t.Errorf("GET /me/quota with no allowance configured: got %d, want 404", code)
	}
	code, body := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/runs/preflight"+
		"?version_id="+f.versionID+"&test_case_id="+f.testCaseID, "")
	if code != http.StatusOK {
		t.Fatalf("GET preflight: got %d", code)
	}
	if _, present := body["quota"]; present {
		t.Error("the pre-run summary carries a quota block in a build that enforces none")
	}
}

func TestPreflightCarriesTheQuotaOutsideTheHash(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 5, Window: 30, FirstWindow: 20, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-quota-preflight")

	code, before := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight: got %d (%s)", code, before.Error)
	}
	_, body := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/runs/preflight"+
		"?version_id="+f.versionID+"&test_case_id="+f.testCaseID, "")
	quota, present := body["quota"].(map[string]any)
	if !present {
		t.Fatalf("the pre-run summary carries no quota block: %v", body)
	}
	if quota["remaining_today"] != float64(5) {
		t.Errorf("remaining_today on the summary is %v, want 5", quota["remaining_today"])
	}

	seedCountedRun(t, pool, f, "", time.Now().Add(-time.Hour))
	code, after := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight after a run: got %d", code)
	}
	if after.Hash != before.Hash {
		t.Errorf("the summary hash changed when the allowance did: %s -> %s", before.Hash, after.Hash)
	}
}

func TestAdmissionListGatesForkRunAndDownloadOnly(t *testing.T) {
	pool := requireDB(t)

	a := betaAPI(t, pool, policy.QuotaLimits{}, []string{"alice-invited"}, 0)
	alice := newFixture(t, a, pool, "alice-invited")
	bob := newFixture(t, a, pool, "bob-uninvited")

	for _, path := range []string{
		"/api/skills/search?q=summarise+a+csv",
		"/api/skills/" + bob.skillID,
	} {
		if code := bob.status(t, http.MethodGet, path); code != http.StatusOK {
			t.Errorf("GET %s as an uninvited user: got %d, want 200", path, code)
		}
	}

	code, body := bob.doJSON(t, http.MethodPost, "/skills/"+bob.skillID+"/fork", `{}`)
	if code != http.StatusForbidden {
		t.Errorf("POST fork as an uninvited user: got %d, want 403", code)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "closed beta") {
		t.Errorf("the refusal does not explain itself: %v", body)
	}
	hash := bob.confirmPermissions(t)
	if code, _ := bob.startWithHash(t, hash); code != http.StatusForbidden {
		t.Errorf("POST run as an uninvited user: got %d, want 403", code)
	}

	const noSuchArtifact = "/downloads/00000000-0000-4000-8000-0000000000fe/content"
	for _, gated := range []struct{ name, method, path, body string }{
		{"POST packaging", http.MethodPost, packagingPath(bob.skillID, bob.versionID), `{"target":"claude-code"}`},
		{"GET download content", http.MethodGet, noSuchArtifact, ""},
	} {
		code, body := bob.doJSON(t, gated.method, gated.path, gated.body)
		if code != http.StatusForbidden {
			t.Errorf("%s as an uninvited user: got %d, want 403", gated.name, code)
		}

		if msg, _ := body["error"].(string); !strings.Contains(msg, "closed beta") {
			t.Errorf("%s refused an uninvited user for some other reason: %v", gated.name, body)
		}
	}

	if code, _ := alice.doJSON(t, http.MethodPost, "/skills/"+alice.skillID+"/fork", `{}`); code != http.StatusCreated {
		t.Errorf("POST fork as an invited user: got %d, want 201", code)
	}

	if code := alice.status(t, http.MethodGet, noSuchArtifact); code != http.StatusNotFound {
		t.Errorf("GET download content as an invited user: got %d, want 404", code)
	}

	if code, body := alice.doJSON(t, http.MethodPost,
		packagingPath(alice.skillID, alice.versionID), `{"target":"claude-code"}`); code == http.StatusForbidden {
		t.Errorf("POST packaging as an invited user: got 403 (%v); the gate refused somebody on the list", body)
	}
}

func TestAdmissionListGatesCreationSessionsAsWell(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Auth.Invited = map[string]bool{"alice-creation-invited": true}
		d.GenerateExposed = true
		d.CreationExposed = true
	})
	alice := newFixture(t, a, pool, "alice-creation-invited")
	bob := newFixture(t, a, pool, "bob-creation-uninvited")

	code, body := bob.doJSON(t, http.MethodPost, "/creation-sessions",
		`{"id":"00000000-0000-4000-8000-000000000301","message":"x","budget_credits":650}`)
	if code != http.StatusForbidden {
		t.Errorf("POST /creation-sessions as an uninvited user: got %d, want 403", code)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "closed beta") {
		t.Errorf("the refusal does not explain itself: %v", body)
	}

	if code, body := alice.doJSON(t, http.MethodPost, "/creation-sessions",
		`{"id":"00000000-0000-4000-8000-000000000302","message":"x","budget_credits":650}`); code == http.StatusForbidden {
		t.Errorf("POST /creation-sessions as an invited user: got 403 (%v); the gate refused somebody on the list", body)
	}
}

func TestNoAdmissionListMeansNoGate(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "alice-no-allowlist")

	if code, _ := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/fork", `{}`); code != http.StatusCreated {
		t.Errorf("POST fork with no allowlist configured: got %d, want 201", code)
	}
}

func TestTheFourFunnelEventsAreEmitted(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-funnel")

	session := f.analyticsSession(t)
	if session == "" {
		t.Fatal("no analytics session cookie was issued")
	}

	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'session_started' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("session_started events for this visitor: %d, want 1", n)
	}

	if code := f.status(t, http.MethodGet, "/api/skills/search?q=summarise+a+csv+file"); code != http.StatusOK {
		t.Fatalf("public search: got %d", code)
	}
	var length int
	var language string
	var hasResults bool
	err := pool.QueryRow(context.Background(), `
		SELECT query_length, query_language, has_results FROM analytics_events
		WHERE event_name = 'search_performed' AND session_id = $1`, session,
	).Scan(&length, &language, &hasResults)
	if err != nil {
		t.Fatalf("no search_performed event: %v", err)
	}
	if want := len("summarise a csv file"); length != want {
		t.Errorf("query_length is %d, want %d", length, want)
	}
	if language != "latin" {
		t.Errorf("query_language is %q, want latin", language)
	}

	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'search_performed' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("search_performed events for one search: %d, want 1", n)
	}

	if code := f.status(t, http.MethodGet, "/api/skills/"+f.skillID); code != http.StatusOK {
		t.Fatalf("skill detail: got %d", code)
	}
	var viewedSkill string
	if err := pool.QueryRow(context.Background(), `
		SELECT skill_id FROM analytics_events
		WHERE event_name = 'skill_detail_viewed' AND session_id = $1`, session,
	).Scan(&viewedSkill); err != nil {
		t.Fatalf("no skill_detail_viewed event: %v", err)
	}
	if viewedSkill != f.skillID {
		t.Errorf("skill_detail_viewed records skill %q, want %q", viewedSkill, f.skillID)
	}

	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'skill_detail_viewed' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("skill_detail_viewed events for one detail read: %d, want 1", n)
	}

	if code := f.status(t, http.MethodGet, "/api/skills/"+f.skillID+"?view=embedded"); code != http.StatusOK {
		t.Fatalf("embedded skill read: got %d", code)
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'skill_detail_viewed' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("an embedded read was counted as opening a skill: %d events, want 1", n)
	}

	f.status(t, http.MethodGet, "/downloads/00000000-0000-4000-8000-0000000000ff/content")
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'download_started' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("download_started events: %d, want 1", n)
	}

	if n := betaCount(t, pool, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'analytics_events' AND column_name IN ('query', 'query_text', 'message', 'payload')`,
	); n != 0 {
		t.Error("analytics_events has grown a column that can hold free text")
	}
}

func TestNoFunnelEventsWithoutARetentionPeriod(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)

	before := betaCount(t, pool, `SELECT count(*) FROM analytics_events`)
	f := newFixture(t, a, pool, "alice-no-analytics")

	if code := f.status(t, http.MethodGet, "/api/skills/search?q=anything+at+all"); code != http.StatusOK {
		t.Fatalf("public search: got %d", code)
	}
	if code := f.status(t, http.MethodGet, "/api/skills/"+f.skillID); code != http.StatusOK {
		t.Fatalf("skill detail: got %d", code)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM analytics_events`); n != before {
		t.Errorf("a deployment with no retention period collected %d events", n-before)
	}
	for _, c := range f.Jar.Cookies(mustURL(t, f.base)) {
		if c.Name == "sh_analytics" {
			t.Error("a deployment with no retention period still set an analytics cookie")
		}
	}
}

func TestAnalyticsSessionIsNotTheSessionToken(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-session-ids")

	var session, funnel string
	for _, c := range f.Jar.Cookies(mustURL(t, f.base)) {
		switch c.Name {
		case "sh_session":
			session = c.Value
		case "sh_analytics":
			funnel = c.Value
		}
	}
	if session == "" || funnel == "" {
		t.Fatalf("expected both cookies, got session set=%t analytics set=%t", session != "", funnel != "")
	}
	if strings.Contains(session, funnel) || strings.Contains(funnel, session) {
		t.Error("the analytics session id and the session token share material")
	}
	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT session_id FROM analytics_events ORDER BY occurred_at LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM sessions WHERE encode(token_hash, 'hex') = $1`, stored); n != 0 {
		t.Error("the stored analytics session id resolves to a sessions row")
	}
}

func TestPurgeDetachesAnalyticsAndFeedbackWithoutDeletingThem(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-purge-analytics")

	if code := f.status(t, http.MethodGet, "/api/skills/"+f.skillID); code != http.StatusOK {
		t.Fatalf("skill detail: got %d", code)
	}
	if code, body := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"I wanted a python profile"}`); code != http.StatusNoContent {
		t.Fatalf("POST /feedback: got %d, body %v", code, body)
	}
	ws := mustUUID(t, f.workspaceID)
	events := betaCount(t, pool, `SELECT count(*) FROM analytics_events WHERE workspace_id = $1`, ws)
	if events == 0 {
		t.Fatal("no workspace-attributed analytics event to detach")
	}

	if code := f.status(t, http.MethodDelete, "/me"); code != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", code)
	}
	if _, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), a.packages, 0, 10); err != nil {
		t.Fatal(err)
	}

	if n := betaCount(t, pool, `SELECT count(*) FROM analytics_events WHERE workspace_id = $1`, ws); n != 0 {
		t.Errorf("%d analytics events still name the purged workspace", n)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM analytics_events`); n < events {
		t.Errorf("the purge deleted analytics events instead of de-identifying them")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE workspace_id IS NULL AND user_id IS NULL`); n == 0 {
		t.Error("the beta feedback was deleted rather than de-identified")
	}
}

func TestFeedbackIsRecordedWithWorkspaceScope(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	alice := newFixture(t, a, pool, "alice-feedback")
	bob := newFixture(t, a, pool, "bob-feedback")
	bobRun := bob.start(t)

	body := fmt.Sprintf(
		`{"kind":"blocking_issue","message":"the run page never left queued","page_path":"/runs/x","run_id":%q,"build_id":"abc123def456"}`,
		bobRun.RunID)
	if code, out := alice.doJSON(t, http.MethodPost, "/feedback", body); code != http.StatusNoContent {
		t.Fatalf("POST /feedback: got %d, body %v", code, out)
	}

	var kind, path string
	var runID, buildID *string
	var workspace string
	err := pool.QueryRow(context.Background(), `
		SELECT kind, page_path, run_id::text, workspace_id::text, build_id FROM feedback_reports
		ORDER BY created_at DESC LIMIT 1`).Scan(&kind, &path, &runID, &workspace, &buildID)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "blocking_issue" || path != "/runs/x" {
		t.Errorf("stored kind=%q page_path=%q", kind, path)
	}

	if buildID == nil || *buildID != "abc123def456" {
		t.Errorf("build_id was not stored on the report: %v", buildID)
	}
	if workspace != alice.workspaceID {
		t.Errorf("the report was filed against workspace %s, want the session's %s", workspace, alice.workspaceID)
	}

	if runID != nil {
		t.Errorf("a run from another workspace was stored on the report: %v", *runID)
	}
}

func TestFeedbackRunLookupFailureOnlyDropsTheOptionalRunID(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Analytics.Svc.RunBelongsToWorkspace = func(context.Context, pgtype.UUID, pgtype.UUID) (bool, error) {
			return false, errors.New("lookup unavailable")
		}
	})
	f := newFixture(t, a, pool, "feedback-run-lookup-error")
	body := `{"kind":"need_signal","message":"lookup error must not lose this report","run_id":"00000000-0000-0000-0000-000000000001"}`
	if code, out := f.doJSON(t, http.MethodPost, "/feedback", body); code != http.StatusNoContent {
		t.Fatalf("POST /feedback: got %d, body %v", code, out)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM feedback_reports
		WHERE message = 'lookup error must not lose this report' AND run_id IS NULL`); n != 1 {
		t.Errorf("feedback with failed optional lookup stored %d times without run_id, want 1", n)
	}
}

func TestFeedbackWithRunIDFailsClosedWithoutTheOwnerReader(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Analytics.Svc.RunBelongsToWorkspace = nil
	})
	f := newFixture(t, a, pool, "feedback-run-reader-missing")
	body := `{"kind":"need_signal","message":"must not be stored without wiring","run_id":"00000000-0000-0000-0000-000000000001"}`
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback", body); code != http.StatusInternalServerError {
		t.Fatalf("POST /feedback without owner reader: got %d, want 500", code)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM feedback_reports
		WHERE message = 'must not be stored without wiring'`); n != 0 {
		t.Errorf("feedback was stored %d times without the owner reader", n)
	}
}

func TestFeedbackRejectsWhatTheContractRejects(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "alice-feedback-validation")

	cases := map[string]string{
		"unknown kind":  `{"kind":"praise","message":"nice"}`,
		"blank message": `{"kind":"need_signal","message":"   "}`,
		"no kind":       `{"message":"something"}`,
		"oversized":     `{"kind":"need_signal","message":"` + strings.Repeat("x", 2001) + `"}`,
	}
	for name, body := range cases {
		if code, _ := f.doJSON(t, http.MethodPost, "/feedback", body); code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, code)
		}
	}

	if code, _ := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"ok","page_path":"https://host/search?q=my+secret"}`); code != http.StatusNoContent {
		t.Fatal("a report with an unusable page_path was refused instead of accepted without it")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE page_path IS NOT NULL AND page_path LIKE '%?%'`); n != 0 {
		t.Error("a page_path with a query string reached the table")
	}

	messageAtCap := strings.Repeat("x", 2000)
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"`+messageAtCap+`"}`); code != http.StatusNoContent {
		t.Fatal("a message at exactly the 2000-rune cap was refused")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE message = $1`, messageAtCap); n != 1 {
		t.Errorf("a message at exactly the 2000-rune cap was not stored: %d rows", n)
	}

	pathAtCap := "/" + strings.Repeat("p", 511)
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"path at the cap","page_path":"`+pathAtCap+`"}`); code != http.StatusNoContent {
		t.Fatal("a page_path at exactly the 512-character cap was refused")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE message = 'path at the cap' AND page_path = $1`,
		pathAtCap); n != 1 {
		t.Errorf("a page_path at exactly the 512-character cap was not stored verbatim: %d rows", n)
	}

	pathOverCap := "/" + strings.Repeat("p", 512)
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"path over the cap","page_path":"`+pathOverCap+`"}`); code != http.StatusNoContent {
		t.Fatal("a page_path one character over the cap was refused instead of dropped")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE message = 'path over the cap' AND page_path IS NULL`); n != 1 {
		t.Errorf("a page_path over the 512-character cap was not dropped: %d rows", n)
	}
}
