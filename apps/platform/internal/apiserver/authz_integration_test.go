// Package apiserver_test holds the database-backed tests that exercise the whole
// wired HTTP surface — the object graph apiserver.NewApp builds and the route
// table apiserver.NewRouter declares, not a copy of either. It is an external
// test package so it can construct that graph without an import cycle, which is
// also why the DISC-001/002 search and WS-001 fork tests live here (see
// disc_integration_test.go) rather than beside the packages they exercise.
//
// They used to live in internal/identity for the same import-cycle reason, which
// made twenty-three files about runs, packaging, evaluation and tracing look like
// identity tests. They test the API; they live beside the API (DDD-011).
//
// This file covers CORE-005 (login, logout, workspace access control) and
// CORE-006 (private content authorization), and carries the harness the rest of
// the package shares.
//
// These tests need a throwaway PostgreSQL with the pgvector extension
// available. Point SKILLHUB_TEST_DATABASE_URL at one and they run; leave it
// unset and they skip, so a CI job without a database reports "skipped" rather
// than a false failure.
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.
package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/eval"
	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/packaging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trace"
)

const dbURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(dbURLEnv)
	if dsn == "" {
		os.Exit(m.Run()) // every test skips; see requireDB
	}
	if err := validateDestructiveTestDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migrate(ctx, pool); err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

func validateDestructiveTestDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", dbURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", dbURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", dbURLEnv)
	}
	return nil
}

func TestDestructiveTestDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
		"postgres://user:pass@localhost/postgres",
	} {
		if err := validateDestructiveTestDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructiveTestDatabaseURL("postgres://user:pass@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func TestConcurrentFirstLoginCreatesOneAccount(t *testing.T) {
	pool := requireDB(t)
	svc := &identity.Service{Pool: pool}
	id := identity.ExternalIdentity{
		Provider: "github", ProviderUserID: "concurrent-first-login",
		Email: "concurrent-first-login@example.test", Name: "Concurrent", Login: "concurrent",
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			_, err := svc.LoginOrSignup(context.Background(), id)
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent LoginOrSignup() failed: %v", err)
		}
	}

	var users, identities, workspaces int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE email = $1", id.Email).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM user_identities WHERE provider = $1 AND provider_user_id = $2", id.Provider, id.ProviderUserID).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM workspaces w JOIN users u ON u.id = w.owner_user_id WHERE u.email = $1`, id.Email).Scan(&workspaces); err != nil {
		t.Fatal(err)
	}
	if users != 1 || identities != 1 || workspaces != 1 {
		t.Fatalf("first login created users=%d identities=%d workspaces=%d, want 1/1/1", users, identities, workspaces)
	}
}

// migrate resets the schema and applies db/migrations in filename order, which
// is also the proof that the migration set applies cleanly from empty.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		// Exec with no arguments uses the simple protocol, so a migration file
		// with several statements applies as one batch.
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return err
		}
	}
	// River owns its own tables and applies them itself (see the header of
	// db/migrations/0016), so the schema is only complete once this has run too.
	return queue.EnsureSchema(ctx, pool)
}

func requireDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skipf("%s not set; skipping database-backed authorization test", dbURLEnv)
	}
	return testPool
}

// api is the object graph cmd/api serves — apiserver.NewApp, not a copy of it —
// so a route that loses its RequireSession here loses it in production too, a
// dependency left unwired here is unwired there, and every route in the table is
// reachable from a test.
type api struct {
	*httptest.Server
	auth *identity.Handler
	// packages is the object store behind the detail and file views; a test
	// seeds a real zip into it under the version's package_object_key.
	packages packageStore
	// runs is the API's run service, exposed so a test can point it at a fake
	// sandbox provider (RUN-005 refuses incompatible work before queueing, and
	// that refusal happens on this side).
	runs *run.Service
	// traceSigner mints ingestion tokens, so a trace test can post as the
	// execution plane would (TRACE-002).
	traceSigner *trace.Signer
	// evaluations is the API's read-only evaluation service, exposed so an
	// EVAL-001 test can produce a verdict with a fake judge (the API never does).
	evaluations *eval.Service
	// packaging is the API's packaging service, exposed so a PACK-001 test can
	// build twice without the idempotent endpoint answering the second call with
	// the first call's artifact.
	packaging *packaging.Service
	// handler is the same route table the server above serves. Kept so a test
	// that needs the API reachable from *another container* — the real end to
	// end run, whose sandbox provider pushes trace events back — can serve it on
	// an address that is not 127.0.0.1.
	handler http.Handler
}

func newAPI(t *testing.T, pool *pgxpool.Pool) *api {
	t.Helper()
	return newAPIWithLLM(t, pool, "")
}

// newAPIWithLLM is newAPI with the internal LLM service pointed somewhere. An
// empty base URL leaves the client nil, which is the "embedding service not
// configured" branch of the DISC-001 degradation path.
func newAPIWithLLM(t *testing.T, pool *pgxpool.Pool, llmBaseURL string) *api {
	t.Helper()
	return newAPITuned(t, pool, llmBaseURL, nil)
}

// newAPITuned is newAPIWithLLM with one hook: tune runs on the assembled Deps
// immediately before the route table is built. It exists because the closed-beta
// features are configuration rather than code paths — the run allowance, the
// invite list and the analytics retention are all deployment settings, and two of
// them change which routes exist at all — so a test has to be able to set them
// before the route table reads them, not after.
//
// The object graph itself is apiserver.NewApp's, not a copy of it (ADR-032 §5):
// what used to be a hand-written transcription of cmd/api's wiring here is now
// the same constructor cmd/api calls, so a dependency that goes missing in
// production goes missing in these tests too. Only the deployment inputs below
// differ from production's.
func newAPITuned(
	t *testing.T, pool *pgxpool.Pool, llmBaseURL string, tune func(*apiserver.Deps),
) *api {
	t.Helper()
	// packageStore is the per-test object store: empty unless a test seeds a
	// package into it, which is also the "stored package unreadable" path the
	// detail view has to survive without claiming a clean scan.
	packages := packageStore{}
	var llm *llmclient.Client
	if llmBaseURL != "" {
		llm = &llmclient.Client{BaseURL: llmBaseURL}
	}
	// The real profile files rather than fixtures: a copy would keep these tests
	// green while a profile edit changed produced packages.
	profiles, err := packaging.LoadProfiles(filepath.Join("..", "..", "..", "..", "contracts", "packaging", "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	// A fixed secret: the tests mint their own ingestion tokens against it, which
	// is exactly what the worker does when it builds a RunRequest.
	traceSigner := &trace.Signer{Secret: []byte("integration-test-trace-secret")}

	app, err := apiserver.NewApp(apiserver.Config{
		Pool:    pool,
		Store:   packages,
		LLM:     llm,
		Fetcher: &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts()},

		TraceSigner:       traceSigner,
		Profiles:          profiles,
		DownloadRetention: 24 * time.Hour,
		// Zero retention means this "deployment" collects no funnel events, which
		// is the shipped state until PDM-006 ratifies a period (ADR-029 決策 5). A
		// beta test turns it on through tune.
		AnalyticsRetention: 0,

		// Zero-value OAuth: no request in these tests reaches GitHub, but
		// GET /auth/github/login builds an authorize URL from it.
		OAuth:    &identity.GitHubOAuth{},
		Secure:   false, // httptest speaks plain http
		DevLogin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tune != nil {
		tune(&app.Deps)
	}
	handler := app.Handler()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &api{
		Server: srv, auth: app.Auth, packages: packages, runs: app.RunSvc,
		traceSigner: traceSigner, handler: handler, evaluations: app.EvalSvc,
		packaging: app.PackagingSvc,
	}
}

// client is one logged-in browser: its jar carries exactly one user's session.
type client struct {
	*http.Client
	base        string
	workspaceID string
	userID      string
}

// login signs in through the offline dev provider, which stands in for GitHub
// OAuth without leaving the machine (ADR-020).
func (a *api) login(t *testing.T, name string) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{Client: &http.Client{Jar: jar}, base: a.URL}
	body := strings.NewReader(`{"user":"` + name + `"}`)
	resp, err := c.Post(a.URL+"/auth/dev/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dev login for %s: got %d", name, resp.StatusCode)
	}
	me := c.me(t)
	c.userID, c.workspaceID = me["user_id"], me["workspace_id"]
	if c.workspaceID == "" {
		t.Fatalf("login for %s produced no workspace", name)
	}
	return c
}

func (c *client) me(t *testing.T) map[string]string {
	t.Helper()
	resp, err := c.Get(c.base + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me: got %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (c *client) status(t *testing.T, method, path string) int {
	t.Helper()
	req, err := http.NewRequest(method, c.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (c *client) skillIDs(t *testing.T, path string) []string {
	t.Helper()
	resp, err := c.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: got %d", path, resp.StatusCode)
	}
	// "skills" is the registry list shape, "results" the search shape; a test
	// only ever cares which ids came back.
	var out struct {
		Skills  []skillRef `json:"skills"`
		Results []skillRef `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(out.Skills)+len(out.Results))
	for _, s := range append(out.Skills, out.Results...) {
		ids = append(ids, s.SkillID)
	}
	return ids
}

type skillRef struct {
	SkillID string `json:"skill_id"`
}

// seedSkill inserts a skill straight into a workspace and indexes it, so a
// test can own content in a workspace it never authenticates as.
func seedSkill(t *testing.T, pool *pgxpool.Pool, workspaceID, name string) string {
	t.Helper()
	ctx := context.Background()
	var ws pgtype.UUID
	if err := ws.Scan(workspaceID); err != nil {
		t.Fatal(err)
	}
	summary := name + " summary"
	q := gen.New(pool)
	skill, err := q.CreateSkill(ctx, gen.CreateSkillParams{
		WorkspaceID: ws, Name: name, Summary: &summary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
		SkillID: skill.ID, WorkspaceID: ws, Name: name, Summary: summary,
	}); err != nil {
		t.Fatal(err)
	}
	id, _ := skill.ID.Value()
	s, _ := id.(string)
	return s
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// The anonymous authorization matrix that used to live here — a hand-picked ~30
// of the table's 68 routes — is now the complete one in
// authz_matrix_integration_test.go, held complete by a scan of the route table's
// own source.

// CORE-005: a session is what login hands out and logout takes away.
func TestLoginLogoutSessionLifecycle(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	if got := (&client{Client: http.DefaultClient, base: a.URL}).status(t, http.MethodGet, "/me"); got != http.StatusUnauthorized {
		t.Fatalf("GET /me without a session: want 401, got %d", got)
	}

	alice := a.login(t, "alice-lifecycle")
	if alice.me(t)["workspace_id"] != alice.workspaceID {
		t.Fatal("GET /me returned a different workspace on the second call")
	}

	// The token must never be readable by page scripts (ADR-020).
	var session *http.Cookie
	for _, c := range alice.Jar.Cookies(mustURL(t, a.URL)) {
		if c.Name == "sh_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("login set no session cookie")
	}

	resp, err := alice.Post(a.URL+"/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: got %d", resp.StatusCode)
	}

	// Revocation is server-side: replaying the old token must fail even though
	// the client still holds it. This is the property JWTs would not give us.
	req, _ := http.NewRequest(http.MethodGet, a.URL+"/me", nil)
	req.AddCookie(session)
	replay, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replaying a logged-out token: want 401, got %d", replay.StatusCode)
	}
}

// CORE-005: an expired session is refused by the read path, not just by the
// cleanup job, and the cleanup job then removes the row idempotently.
func TestExpiredSessionRejectedThenCleaned(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	bob := a.login(t, "bob-expiry")

	ctx := context.Background()
	var uid pgtype.UUID
	if err := uid.Scan(bob.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE sessions SET expires_at = now() - interval '1 hour' WHERE user_id = $1", uid,
	); err != nil {
		t.Fatal(err)
	}

	if got := bob.status(t, http.MethodGet, "/me"); got != http.StatusUnauthorized {
		t.Fatalf("expired session: want 401, got %d", got)
	}

	n, err := (&identity.Service{Pool: pool}).CleanupExpiredSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("cleanup removed %d expired sessions, want at least 1", n)
	}
	// Idempotent: a second sweep is a no-op, never an error (ADR-008).
	if _, err := (&identity.Service{Pool: pool}).CleanupExpiredSessions(ctx); err != nil {
		t.Fatalf("second cleanup sweep: %v", err)
	}
}

// CORE-006 / WS-002: holding another user's skill id is not access to it.
func TestPrivateContentIsolatedAcrossWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	alice := a.login(t, "alice-isolation")
	bob := a.login(t, "bob-isolation")
	secret := seedSkill(t, pool, alice.workspaceID, "alice-private-widget")

	if ids := alice.skillIDs(t, "/skills"); !contains(ids, secret) {
		t.Fatal("owner cannot see their own skill")
	}
	if ids := bob.skillIDs(t, "/skills"); contains(ids, secret) {
		t.Fatal("another user's skill appears in the caller's list")
	}

	// 404 rather than 403 everywhere: existence itself is private (WS-006).
	for _, tc := range []struct{ method, path string }{
		{http.MethodDelete, "/skills/" + secret},
		{http.MethodPost, "/skills/" + secret + "/fork"},
		{http.MethodGet, "/skills/" + secret + "/diff?from=" + secret + "&to=" + secret},
	} {
		if got := bob.status(t, tc.method, tc.path); got != http.StatusNotFound {
			t.Errorf("%s %s as a non-owner: want 404, got %d", tc.method, tc.path, got)
		}
	}

	// Anonymous callers do not even reach the authorization check.
	anon := &client{Client: http.DefaultClient, base: a.URL}
	if got := anon.status(t, http.MethodDelete, "/skills/"+secret); got != http.StatusUnauthorized {
		t.Errorf("anonymous delete: want 401, got %d", got)
	}

	// Still there: nothing above was allowed to touch it.
	if ids := alice.skillIDs(t, "/skills"); !contains(ids, secret) {
		t.Fatal("a non-owner request modified the owner's skill")
	}
}

// CORE-006 / iron rule 3: scope comes from the session, so a client-supplied
// workspace_id — query string, header, or otherwise — changes nothing.
func TestClientSuppliedWorkspaceIDIsIgnored(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	alice := a.login(t, "alice-forgery")
	bob := a.login(t, "bob-forgery")
	secret := seedSkill(t, pool, alice.workspaceID, "alice-forgery-target")

	forged := []string{
		"/skills?workspace_id=" + alice.workspaceID,
		"/skills?workspace_id=" + alice.workspaceID + "&owner_user_id=" + alice.userID,
		"/skills/search?q=forgery&workspace_id=" + alice.workspaceID,
	}
	for _, path := range forged {
		if ids := bob.skillIDs(t, path); contains(ids, secret) {
			t.Errorf("GET %s leaked another workspace's skill", path)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, a.URL+"/skills", nil)
	req.Header.Set("X-Workspace-Id", alice.workspaceID)
	resp, err := bob.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Skills []skillRef `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, s := range out.Skills {
		if s.SkillID == secret {
			t.Fatal("X-Workspace-Id header widened the caller's scope")
		}
	}
}

// CORE-006 / DISC-010: public search is open to anonymous callers, so it must
// answer from the public catalog only — never from private workspaces.
func TestPublicSearchSeesOnlyCatalogWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()

	alice := a.login(t, "alice-search")
	private := seedSkill(t, pool, alice.workspaceID, "zaphodian private analyzer")

	curator := a.login(t, "curator-search")
	var curatorWS pgtype.UUID
	if err := curatorWS.Scan(curator.workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE workspaces SET is_catalog = true WHERE id = $1", curatorWS); err != nil {
		t.Fatal(err)
	}
	published := seedSkill(t, pool, curator.workspaceID, "zaphodian public analyzer")

	anon := &client{Client: http.DefaultClient, base: a.URL}
	ids := anon.skillIDs(t, "/api/skills/search?q=zaphodian")
	if contains(ids, private) {
		t.Fatal("public search exposed a private workspace's skill")
	}
	if !contains(ids, published) {
		t.Fatalf("public search missed the catalog skill; got %v", ids)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// lockTestSchema serialises the packages that reset this database.
//
// apiserver, eval and registry each drop and recreate schema "public" in
// SKILLHUB_TEST_DATABASE_URL, and `go test ./...` runs packages concurrently:
// one package's reset lands while another is mid-run, and the second one sees
// "relation does not exist". Held on one connection for the whole package run
// rather than only across the migration, because the hazard is a reset
// colliding with somebody else's *tests*, not with their migration.
//
// Session-scoped, so a crashed run releases it along with its connection and a
// stale lock cannot wedge CI. Every package that resets this database must take
// it; one that forgets fails loudly with the panic above rather than silently.
func lockTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	return func() {
		_, _ = conn.Exec(ctx,
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	}
}
