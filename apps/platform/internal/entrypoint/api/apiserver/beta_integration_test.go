// Closed-beta integration tests: the PDM-010 run allowance (ADR-028 決策 2), the
// BETA-001 admission list (ADR-028 決策 1), the O11Y-004 funnel events and the
// BETA-003/004/005 feedback channel (ADR-029).
//
// They live in apiserver_test with the rest of the database-backed HTTP tests so
// they serve the real route table from apiserver.NewRouter rather than a copy —
// which matters more here than anywhere else, because two of the three features
// are configuration that changes which routes exist at all.
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

// --- helpers -----------------------------------------------------------------

// betaAPI is newAPI with the closed-beta knobs set before the route table is
// built. Any of them may be zero, which is the shipped default for all three.
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

// seedCountedRun writes a run that has already been through `preparing`, which is
// what PDM-010 counts. Straight into the database rather than through the API: the
// point of these tests is what the counter counts, and driving a real run to
// `preparing` would need a sandbox provider.
//
// failureClass may be empty (the run has not failed) or any value 0018 allows.
func seedCountedRun(t *testing.T, pool *pgxpool.Pool, f fixture, failureClass string, at time.Time) {
	t.Helper()
	// Terminal on purpose: a counted run must not also hold a concurrency slot, or
	// these tests would be measuring the wrong ceiling.
	status, class := "succeeded", any(nil)
	if failureClass != "" {
		status, class = "failed", any(failureClass)
	}
	seedBetaRun(t, pool, f, status, class, &at)
}

// seedBetaRun writes one run row directly, with an optional `preparing`
// transition at a chosen time.
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

// analyticsSession reads the funnel cookie this browser is carrying, so an
// assertion can be scoped to one visitor rather than to a table every test in the
// package shares.
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

// --- PDM-010: the allowance is enforced --------------------------------------

// The refusal itself, and the shape 02 asks for: 422 (well formed, platform fine,
// may not proceed), and no run left behind.
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

	// The check runs inside the transaction that would have inserted the run, so a
	// refusal rolls the whole thing back. This assertion is the "同一個交易" half of
	// ADR-028 決策 2 — a check in the handler could refuse after a row existed.
	if after := betaCount(t, pool, `SELECT count(*) FROM runs WHERE workspace_id = $1`, mustUUID(t, f.workspaceID)); after != before {
		t.Errorf("a refused run left %d rows behind", after-before)
	}
}

// The window ceiling, and the first-window value PDM-010 proposes. A brand new
// workspace is inside its first window, so the lower of the two applies.
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

// PDM-010's refund list, as the predicate it is (ADR-028 決策 2). Platform-side
// terminations do not count; the skill failing at its own job, a user cancelling
// and a run that used all its time do.
//
// Written as a table over the whole 0018 vocabulary rather than over the three
// interesting values: a class added later and forgotten would silently start
// costing users their allowance, and the compiler cannot catch that.
func TestOnlyPlatformSideFailuresAreRefunded(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 100, Window: 100, FirstWindow: 100, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)

	cases := []struct {
		failureClass string
		counted      bool
	}{
		{"provider_error", false},      // the provider could not carry it
		{"platform_error", false},      // our own fault
		{"capability_mismatch", false}, // gate B's baseline refused it
		{"workload_error", true},       // the skill failed at its own job
		{"cancelled", true},            // the user asked
		{"timeout", true},              // it had its full time and spent it
		{"", true},                     // it succeeded
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

// A run that never reached `preparing` never counted, so there is nothing to
// refund (PDM-010's own inference from where counting starts).
func TestRunsThatNeverReachedPreparingDoNotCount(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 10, Window: 10, FirstWindow: 10, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-quota-provisioning")

	// A real run through the API: it is created `queued` and, with no provider
	// configured, fails without ever preparing.
	f.start(t)

	_, q := f.quota(t)
	if q.RemainingWindow != 10 {
		t.Errorf("a run that never prepared spent %d of the allowance", 10-q.RemainingWindow)
	}
}

// The advisory lock in the create transaction (gateb.go, and the allowance now
// rides on it): two simultaneous creates on a workspace with one slot left must
// produce one run, not two.
//
// This is the concurrency ceiling rather than the allowance, and deliberately so —
// under PDM-010's counting rule a queued run is not counted yet, so the allowance
// cannot be raced in the same way, and this is the bound that keeps that gap at
// one run (see requireQuota's comment). Testing the lock that actually holds is
// worth more than a test that asserts a bound nothing enforces.
func TestTwoSimultaneousRunsCannotBothTakeTheLastSlot(t *testing.T) {
	pool := requireDB(t)
	limits := policy.QuotaLimits{Daily: 50, Window: 50, FirstWindow: 50, WindowDays: 30}
	a := betaAPI(t, pool, limits, nil, 0)
	f := newFixture(t, a, pool, "alice-concurrency-race")

	// One slot of MaxConcurrentRunsPerWorkspace already taken by a non-terminal run.
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

// Enforcement first, display second (ADR-028 決策 3, 04 乙-2). A deployment with no
// allowance shows no allowance — not zeroes, not the route.
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

// And where it is enforced, the summary carries it — outside the hash, by the rule
// TEST-011 set for estimated_cost: an allowance is a state, not a permission.
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

	// Spending allowance must not invalidate a confirmation somebody is holding.
	seedCountedRun(t, pool, f, "", time.Now().Add(-time.Hour))
	code, after := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight after a run: got %d", code)
	}
	if after.Hash != before.Hash {
		t.Errorf("the summary hash changed when the allowance did: %s -> %s", before.Hash, after.Hash)
	}
}

// --- BETA-001: the admission list --------------------------------------------

// ADR-028 決策 1's table, as a test: search and detail stay open to everybody, and
// fork, run creation and download close.
func TestAdmissionListGatesForkRunAndDownloadOnly(t *testing.T) {
	pool := requireDB(t)
	// alice is invited; bob is not. Keyed by provider_user_id, which for the dev
	// provider is the login name.
	a := betaAPI(t, pool, policy.QuotaLimits{}, []string{"alice-invited"}, 0)
	alice := newFixture(t, a, pool, "alice-invited")
	bob := newFixture(t, a, pool, "bob-uninvited")

	// Open to everybody, invited or not — this was already true (DISC-010) and the
	// gate does not change it.
	for _, path := range []string{
		"/api/skills/search?q=summarise+a+csv",
		"/api/skills/" + bob.skillID,
	} {
		if code := bob.status(t, http.MethodGet, path); code != http.StatusOK {
			t.Errorf("GET %s as an uninvited user: got %d, want 200", path, code)
		}
	}

	// Closed to the uninvited. 403 and not 404: the catalogue has just shown this
	// person the content, so denying the feature exists would be contradicted by
	// their next request (02:SEC-011's /files reasoning).
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

	// And open to the invited: being on the list grants nothing extra, it only
	// stops removing things.
	if code, _ := alice.doJSON(t, http.MethodPost, "/skills/"+alice.skillID+"/fork", `{}`); code != http.StatusCreated {
		t.Errorf("POST fork as an invited user: got %d, want 201", code)
	}
}

// The shipped default. No BETA_ALLOWLIST means no closed beta is running, so every
// signed-in user is admitted — which is what DEV_LOGIN deployments get, and why
// the offline demo path needs no exemption of its own.
func TestNoAdmissionListMeansNoGate(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "alice-no-allowlist")

	if code, _ := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/fork", `{}`); code != http.StatusCreated {
		t.Errorf("POST fork with no allowlist configured: got %d, want 201", code)
	}
}

// --- O11Y-004: the four funnel events ----------------------------------------

// All four, each from the action that produces it, and none of them carrying the
// query text (ADR-029 決策 2).
func TestTheFourFunnelEventsAreEmitted(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-funnel")

	// Everything below is scoped to this browser's analytics session, which is also
	// the assertion ADR-029 決策 4 asks for: one identifier stitches an anonymous
	// first segment to whatever the same person does after signing in.
	session := f.analyticsSession(t)
	if session == "" {
		t.Fatal("no analytics session cookie was issued")
	}

	// session_started: minted with the cookie on the first request of a fresh
	// browser, which the login above already was.
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'session_started' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("session_started events for this visitor: %d, want 1", n)
	}

	// search_performed: length, script, hit count and filter flag; never the words.
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

	// skill_detail_viewed, with the arrival attributes clamped to the whitelist.
	if code := f.status(t, http.MethodGet, "/api/skills/"+f.skillID+"?from=search&rank=2"); code != http.StatusOK {
		t.Fatalf("skill detail: got %d", code)
	}
	var arrival string
	var rank int
	if err := pool.QueryRow(context.Background(), `
		SELECT arrival, arrival_rank FROM analytics_events
		WHERE event_name = 'skill_detail_viewed' AND session_id = $1`, session,
	).Scan(&arrival, &rank); err != nil {
		t.Fatalf("no skill_detail_viewed event: %v", err)
	}
	if arrival != "search" || rank != 2 {
		t.Errorf("arrival is %q rank %d, want search/2", arrival, rank)
	}

	// download_started is the *attempt*, recorded before the handler decides
	// anything — that is the whole reason it exists next to download_records, which
	// only ever holds downloads that succeeded. An artifact that does not exist
	// still produces the event and still 404s.
	f.status(t, http.MethodGet, "/downloads/00000000-0000-4000-8000-0000000000ff/content")
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'download_started' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("download_started events: %d, want 1", n)
	}

	// Whatever else is in the table, no column of it can hold the query text: the
	// schema has no free-text column at all, so this asserts the shape rather than
	// the contents (ADR-029 決策 2).
	if n := betaCount(t, pool, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'analytics_events' AND column_name IN ('query', 'query_text', 'message', 'payload')`,
	); n != 0 {
		t.Error("analytics_events has grown a column that can hold free text")
	}
}

// Nothing is collected until a retention period exists (NFR-002, ADR-029 決策 5).
// Not a row, and not a cookie either.
func TestNoFunnelEventsWithoutARetentionPeriod(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	// Counted as a delta: every test in this package shares one database, so the
	// question is what this API collected, not what the table holds.
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

// ADR-029 決策 4: the analytics session identifier is not the session token and not
// derivable from it. Asserted against the cookies a real browser holds, because
// that is where a reuse would show up.
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

// Account deletion de-identifies rather than deletes (ADR-029 決策 5).
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

// --- BETA-003/004/005: the feedback channel ----------------------------------

func TestFeedbackIsRecordedWithWorkspaceScope(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	alice := newFixture(t, a, pool, "alice-feedback")
	bob := newFixture(t, a, pool, "bob-feedback")
	bobRun := bob.start(t)

	body := fmt.Sprintf(
		`{"kind":"blocking_issue","message":"the run page never left queued","page_path":"/runs/x","run_id":%q}`,
		bobRun.RunID)
	if code, out := alice.doJSON(t, http.MethodPost, "/feedback", body); code != http.StatusNoContent {
		t.Fatalf("POST /feedback: got %d, body %v", code, out)
	}

	var kind, path string
	var runID *string
	var workspace string
	err := pool.QueryRow(context.Background(), `
		SELECT kind, page_path, run_id::text, workspace_id::text FROM feedback_reports
		ORDER BY created_at DESC LIMIT 1`).Scan(&kind, &path, &runID, &workspace)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "blocking_issue" || path != "/runs/x" {
		t.Errorf("stored kind=%q page_path=%q", kind, path)
	}
	if workspace != alice.workspaceID {
		t.Errorf("the report was filed against workspace %s, want the session's %s", workspace, alice.workspaceID)
	}
	// Somebody else's run is dropped, not refused: losing the report over a context
	// field would be the worse outcome, and the 204 tells the caller nothing about
	// whether that run exists (WS-006).
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
	// A full URL where a route path belongs is dropped, not refused: the query
	// string is what must not be stored (beta-design §4.2 界線 2).
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback",
		`{"kind":"need_signal","message":"ok","page_path":"https://host/search?q=my+secret"}`); code != http.StatusNoContent {
		t.Fatal("a report with an unusable page_path was refused instead of accepted without it")
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM feedback_reports WHERE page_path IS NOT NULL AND page_path LIKE '%?%'`); n != 0 {
		t.Error("a page_path with a query string reached the table")
	}
}
