// Package identity_test holds the database-backed tests that need the real HTTP
// surface — identity + registry + catalog wired the way cmd/api wires it. It is
// an external test package so it can do that without an import cycle, which is
// also why the DISC-001/002 search and WS-001 fork tests live here (see
// disc_integration_test.go) rather than beside the packages they exercise.
//
// This file covers CORE-005 (login, logout, workspace access control) and
// CORE-006 (private content authorization).
//
// These tests need a throwaway PostgreSQL with the pgvector extension
// available. Point SKILLHUB_TEST_DATABASE_URL at one and they run; leave it
// unset and they skip, so a CI job without a database reports "skipped" rather
// than a false failure.
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.
package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/registry"
)

const dbURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv(dbURLEnv)
	if url == "" {
		os.Exit(m.Run()) // every test skips; see requireDB
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		panic(err)
	}
	if err := migrate(ctx, pool); err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
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
	return nil
}

func requireDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skipf("%s not set; skipping database-backed authorization test", dbURLEnv)
	}
	return testPool
}

// api mirrors the route wiring in cmd/api: every private route behind
// RequireSession, public search in front of it.
type api struct {
	*httptest.Server
	auth *identity.Handler
	// packages is the object store behind the detail and file views; a test
	// seeds a real zip into it under the version's package_object_key.
	packages packageStore
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
	auth := &identity.Handler{
		Service:  &identity.Service{Pool: pool},
		Secure:   false, // httptest speaks plain http
		DevLogin: true,
	}
	reg := &registry.Handler{Svc: &registry.Service{Pool: pool}, Identity: auth.Service}
	// packageStore is the per-test object store: empty unless a test seeds a
	// package into it, which is also the "stored package unreadable" path the
	// detail view has to survive without claiming a clean scan.
	packages := packageStore{}
	search := &catalog.Handler{Pool: pool, Identity: auth.Service, Store: packages}
	if llmBaseURL != "" {
		search.LLMClient = &llmclient.Client{BaseURL: llmBaseURL}
	}

	mux := http.NewServeMux()
	auth.Mount(mux)
	mux.HandleFunc("GET /api/skills/search", search.PublicSearch)
	mux.HandleFunc("GET /api/skills/{id}", auth.OptionalSession(search.SkillDetail))
	mux.HandleFunc("GET /api/skills/{id}/files", auth.OptionalSession(search.SkillFiles))
	mux.HandleFunc("GET /skills/search", auth.RequireSession(search.Search))
	mux.HandleFunc("GET /skills", auth.RequireSession(reg.List))
	mux.HandleFunc("POST /skills/{id}/fork", auth.RequireSession(reg.Fork))
	mux.HandleFunc("GET /skills/{id}/diff", auth.RequireSession(reg.Diff))
	mux.HandleFunc("DELETE /skills/{id}", auth.RequireSession(reg.Delete))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &api{Server: srv, auth: auth, packages: packages}
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
