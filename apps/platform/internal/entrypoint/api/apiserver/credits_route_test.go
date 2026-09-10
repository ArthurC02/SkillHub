// CRED-001/CRED-007's HTTP surface.
//
// Until 2026-09-10 neither route was mounted: the contract had no operation
// for either, and devctl automation-check's route-table scan fails a route
// mounted ahead of its contract entry (iron rule 12). The contract landed and
// they are mounted, so this file tests two different things:
//
//  1. TestCreditRoutesAreMountedInProduction goes through the real,
//     production NewRouter table and proves both routes are reachable there —
//     the inverse of what this test asserted while the contract was pending,
//     and the assertion that would have caught the mount silently regressing.
//  2. Everything else wires creditsHandler into its OWN httptest mux, through
//     the real auth.RequireSession/RequireOperator wrappers and a real
//     session, to exercise the handler logic and the CRED gating rules
//     against a fake ledger.
//
// Self-contained in this (internal) package rather than apiserver_test,
// because creditsHandler and CreditLedger are unexported. It shares this
// directory's TestMain (migration lives in authz_integration_test.go) and
// needs SKILLHUB_TEST_DATABASE_URL the same way; unset, every test below
// skips.
package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// fakeCreditLedger stands in for the real domain service (out of scope — see
// credits.go). It is deliberately the simplest thing that satisfies
// CreditLedger: an in-memory map keyed by workspace id string.
type fakeCreditLedger struct {
	balances map[string]int64
	estimate CreditSessionEstimate
}

func newFakeCreditLedger(est CreditSessionEstimate) *fakeCreditLedger {
	return &fakeCreditLedger{balances: map[string]int64{}, estimate: est}
}

func (f *fakeCreditLedger) Balance(_ context.Context, workspaceID pgtype.UUID) (int64, error) {
	return f.balances[pgconv.UUIDString(workspaceID)], nil
}

func (f *fakeCreditLedger) SessionEstimate(context.Context) (CreditSessionEstimate, error) {
	return f.estimate, nil
}

func (f *fakeCreditLedger) Grant(_ context.Context, workspaceID pgtype.UUID, amount int64, _ string, _ pgtype.UUID) (int64, error) {
	key := pgconv.UUIDString(workspaceID)
	f.balances[key] += amount
	return f.balances[key], nil
}

// creditsTestPool opens its own connection to the same database
// authz_integration_test.go's TestMain already migrated (one test binary, one
// running process, one schema) rather than sharing that package's unexported
// pool variable, which package apiserver cannot see across the package_test
// boundary.
func creditsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping CRED route test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Both CRED routes are reachable through router.go's real, production table —
// not a copy of it, which is the only version of this assertion worth having.
//
// This test used to assert the exact opposite, and the inversion is the point:
// while the contract was pending it proved the routes were absent on purpose,
// and now it proves they are present. Either way the thing under test is the
// production table, so a future edit that drops a mount turns this red instead
// of quietly removing a balance from the product.
//
// The signed-in user here is not an operator, so the grant route answers 404 —
// the same answer it gives a stranger, which is the whole point of that block
// in router.go. That it is 404 rather than 405 or 401 is what proves the route
// exists AND is operator-gated: an unmounted path and a refused operator call
// are deliberately indistinguishable from outside, so the balance read below
// is what separates "mounted" from "not there at all".
func TestCreditRoutesAreMountedInProduction(t *testing.T) {
	pool := creditsTestPool(t)
	app, err := NewApp(Config{Pool: pool, OAuth: &identity.GitHubOAuth{}, DevLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	app.Deps.Credits = &creditsHandler{
		Ledger: newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65}), Identity: app.Auth.Service,
	}
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	c := creditsLogin(t, srv, "credits-mounted")

	code, body := c.getJSON(t, "/me/credits")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits: got %d, want 200 (mounted since the contract landed)", code)
	}
	if _, ok := body["balance_credits"]; !ok {
		t.Fatalf("GET /me/credits body has no balance_credits: %v", body)
	}
	// ADR-068 decision 1: no field a user is shown is denominated in money.
	// Asserted on the wire rather than trusted from the struct tags, because
	// the tags are what a future edit would change.
	for k := range body {
		if strings.Contains(strings.ToLower(k), "usd") {
			t.Fatalf("GET /me/credits sent a dollar-denominated field %q to a user: %v", k, body)
		}
	}
	if code, _ := c.postJSON(t, "/admin/credits/"+c.workspaceID+"/grants", `{"amount_credits":10,"reason":"x"}`); code != http.StatusNotFound {
		t.Fatalf("POST grant as a non-operator: got %d, want 404", code)
	}
}

// creditsClient is one signed-in browser (mirrors authz_integration_test.go's
// client, duplicated rather than imported for the reason given in the file
// doc comment above).
type creditsClient struct {
	*http.Client
	base        string
	userID      string
	workspaceID string
}

// creditsTestServer wires creditsHandler into its own mux — through the real
// identity.Handler.Mount plus RequireSession/RequireOperator — rather than
// router.go, which does not mount these routes yet (see the file doc
// comment). Once contracts/openapi/public.yaml carries CRED-001/CRED-007, the
// two mux lines below move into router.go verbatim.
func creditsTestServer(t *testing.T, pool *pgxpool.Pool, ledger CreditLedger) (*App, *httptest.Server) {
	t.Helper()
	app, err := NewApp(Config{
		Pool: pool, OAuth: &identity.GitHubOAuth{}, Secure: false, DevLogin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &creditsHandler{Ledger: ledger, Identity: app.Auth.Service}
	app.Deps.Credits = h

	mux := http.NewServeMux()
	app.Auth.Mount(mux)
	mux.HandleFunc("GET /me/credits", app.Auth.RequireSession(h.Get))
	mux.HandleFunc("POST /admin/credits/{workspace_id}/grants", app.Auth.RequireOperator(h.Grant))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return app, srv
}

func creditsLogin(t *testing.T, srv *httptest.Server, name string) *creditsClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &creditsClient{Client: &http.Client{Jar: jar}, base: srv.URL}
	resp, err := c.Post(srv.URL+"/auth/dev/login", "application/json", strings.NewReader(`{"user":"`+name+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dev login for %s: got %d", name, resp.StatusCode)
	}
	me, err := c.Get(srv.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	var body struct {
		UserID      string `json:"user_id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	c.userID = body.UserID
	c.workspaceID = body.WorkspaceID
	return c
}

func (c *creditsClient) getJSON(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *creditsClient) postJSON(t *testing.T, path, body string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Post(c.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// GET /me/credits requires a session, same as every other user-data route on
// this table.
func TestGetCreditsRequiresASession(t *testing.T) {
	pool := creditsTestPool(t)
	_, srv := creditsTestServer(t, pool, newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65}))
	resp, err := http.Get(srv.URL + "/me/credits")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous GET /me/credits: got %d, want 401", resp.StatusCode)
	}
}

// The grant route is 02:SEC-011's non-disclosing operator surface: a signed-in
// member who is not on the operator roster gets the same 404 an unmounted
// route would, not 403.
func TestGrantCreditsIsInvisibleWithoutTheOperatorRole(t *testing.T) {
	pool := creditsTestPool(t)
	_, srv := creditsTestServer(t, pool, newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65}))
	member := creditsLogin(t, srv, "credits-member")

	code, _ := member.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants", `{"amount_credits":10,"reason":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("non-operator grant attempt: got %d, want 404", code)
	}
}

// The whole round trip: a new account starts blocked below the fallback
// threshold (CRED-001's gate ①), an operator grant lifts the balance
// (CRED-007 — the same mechanism a gate-test participant's reward uses), and
// GET /me/credits then reports the session as startable.
func TestOperatorGrantUnblocksANewSession(t *testing.T) {
	pool := creditsTestPool(t)
	ledger := newFakeCreditLedger(CreditSessionEstimate{
		LowCredits: 30, HighCredits: FallbackSessionThresholdCredits,
		ThresholdCredits: FallbackSessionThresholdCredits, SampleSize: 2, Estimated: true,
	})
	app, srv := creditsTestServer(t, pool, ledger)
	member := creditsLogin(t, srv, "credits-member-grant")
	operator := creditsLogin(t, srv, "credits-operator-grant")
	// Live map read at request time (see identity/http.go's RequireOperator), so
	// setting it after the server is already up still takes effect — the same
	// pattern every other operator-route test in this package uses.
	app.Auth.Operators = map[string]bool{operator.userID: true}

	code, before := member.getJSON(t, "/me/credits")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits: got %d, body %v", code, before)
	}
	if canStart, _ := before["can_start"].(bool); canStart {
		t.Fatalf("a fresh account (balance 0) reported can_start = true against threshold %d", FallbackSessionThresholdCredits)
	}
	if reason, _ := before["block_reason"].(string); reason == "" {
		t.Fatal("blocked but block_reason is empty")
	}
	if est, ok := before["estimated_session"].(map[string]any); !ok || est["estimated"] != true {
		t.Fatalf("estimated_session.estimated was not surfaced as true: %v", before["estimated_session"])
	}

	code, granted := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants",
		`{"amount_credits":100,"reason":"beta reward"}`)
	if code != http.StatusOK {
		t.Fatalf("operator grant: got %d, body %v", code, granted)
	}

	code, after := member.getJSON(t, "/me/credits")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits after grant: got %d, body %v", code, after)
	}
	if canStart, _ := after["can_start"].(bool); !canStart {
		t.Fatalf("balance 100 >= threshold %d but can_start is still false: %v", FallbackSessionThresholdCredits, after)
	}
	if reason, _ := after["block_reason"].(string); reason != "" {
		t.Fatalf("can_start but block_reason is still %q", reason)
	}
}
