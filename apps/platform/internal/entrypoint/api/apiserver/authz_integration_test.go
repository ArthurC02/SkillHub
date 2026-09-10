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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

const dbURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(dbURLEnv)
	if dsn == "" {

		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", dbURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
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

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "..", "..", "db", "migrations")
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

		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return err
		}
	}

	return queue.EnsureSchema(ctx, pool)
}

func requireDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skipf("%s not set; skipping database-backed authorization test", dbURLEnv)
	}
	return testPool
}

type api struct {
	*httptest.Server
	auth *identity.Handler

	creditPool      *pgxpool.Pool
	startingCredits int64

	packages packageStore

	runs *run.Service

	traceSigner *trace.Signer

	evaluations *eval.Service

	versions *ingest.Service

	app *apiserver.App

	packaging *packaging.Service

	handler http.Handler
}

func newAPI(t *testing.T, pool *pgxpool.Pool) *api {
	t.Helper()
	return newAPIWithLLM(t, pool, "")
}

func newAPIWithLLM(t *testing.T, pool *pgxpool.Pool, llmBaseURL string) *api {
	t.Helper()
	return newAPITuned(t, pool, llmBaseURL, nil)
}

func newAPITuned(
	t *testing.T, pool *pgxpool.Pool, llmBaseURL string, tune func(*apiserver.Deps),
) *api {
	t.Helper()

	t.Setenv("DEV_LOGIN", "1")

	packages := packageStore{}
	var llm *llmclient.Client
	if llmBaseURL != "" {

		llm = &llmclient.Client{BaseURL: llmBaseURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	}

	profiles, err := packaging.LoadProfiles(filepath.Join("..", "..", "..", "..", "..", "..", "contracts", "packaging", "profiles"))
	if err != nil {
		t.Fatal(err)
	}

	traceSigner := &trace.Signer{Secret: []byte("integration-test-trace-secret")}

	app, err := apiserver.NewApp(apiserver.Config{
		Pool:    pool,
		Store:   packages,
		LLM:     llm,
		Fetcher: &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts()},

		TraceSigner:       traceSigner,
		Profiles:          profiles,
		DownloadRetention: 24 * time.Hour,

		AnalyticsRetention: 0,

		OAuth:    &identity.GitHubOAuth{},
		Secure:   false,
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
		creditPool: pool, startingCredits: betaGrantCredits,
		Server: srv, auth: app.Auth, packages: packages, runs: app.RunSvc,
		traceSigner: traceSigner, handler: handler, evaluations: app.EvalSvc,
		packaging: app.PackagingSvc, versions: app.Versions, app: app,
	}
}

const betaGrantCredits = 13_000

type client struct {
	*http.Client
	base        string
	workspaceID string
	userID      string
}

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
	c.userID, _ = me["user_id"].(string)
	c.workspaceID, _ = me["workspace_id"].(string)
	if c.workspaceID == "" {
		t.Fatalf("login for %s produced no workspace", name)
	}
	if a.startingCredits != 0 && a.creditPool != nil {
		var userID pgtype.UUID
		if err := userID.Scan(c.userID); err != nil {
			t.Fatal(err)
		}
		if _, err := a.creditPool.Exec(context.Background(),
			`INSERT INTO credit_accounts (user_id, balance_credits) VALUES ($1, $2)
			 ON CONFLICT (user_id) DO UPDATE SET balance_credits = EXCLUDED.balance_credits`,
			userID, a.startingCredits); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func (c *client) me(t *testing.T) map[string]any {
	t.Helper()
	resp, err := c.Get(c.base + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me: got %d", resp.StatusCode)
	}
	var out map[string]any
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

	if _, err := pool.Exec(ctx, "UPDATE search_documents SET enrichment_status = 'enriched' WHERE skill_id = $1", skill.ID); err != nil {
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

	if _, err := (&identity.Service{Pool: pool}).CleanupExpiredSessions(ctx); err != nil {
		t.Fatalf("second cleanup sweep: %v", err)
	}
}

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

	for _, tc := range []struct{ method, path string }{
		{http.MethodDelete, "/skills/" + secret},
		{http.MethodPost, "/skills/" + secret + "/fork"},
		{http.MethodGet, "/skills/" + secret + "/diff?from=" + secret + "&to=" + secret},
	} {
		if got := bob.status(t, tc.method, tc.path); got != http.StatusNotFound {
			t.Errorf("%s %s as a non-owner: want 404, got %d", tc.method, tc.path, got)
		}
	}

	anon := &client{Client: http.DefaultClient, base: a.URL}
	if got := anon.status(t, http.MethodDelete, "/skills/"+secret); got != http.StatusUnauthorized {
		t.Errorf("anonymous delete: want 401, got %d", got)
	}

	if ids := alice.skillIDs(t, "/skills"); !contains(ids, secret) {
		t.Fatal("a non-owner request modified the owner's skill")
	}
}

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

	seedEmbedding(t, pool, published, 1301)

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
