package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type fakeCreditLedger struct {
	balances    map[string]int64
	estimate    CreditSessionEstimate
	balanceErr  error
	estimateErr error
}

func newFakeCreditLedger(est CreditSessionEstimate) *fakeCreditLedger {
	return &fakeCreditLedger{balances: map[string]int64{}, estimate: est}
}

func (f *fakeCreditLedger) Balance(_ context.Context, workspaceID pgtype.UUID) (int64, error) {
	if f.balanceErr != nil {
		return 0, f.balanceErr
	}
	return f.balances[pgconv.UUIDString(workspaceID)], nil
}

func (f *fakeCreditLedger) SessionEstimate(context.Context) (CreditSessionEstimate, error) {
	if f.estimateErr != nil {
		return CreditSessionEstimate{}, f.estimateErr
	}
	return f.estimate, nil
}

func (f *fakeCreditLedger) Grant(_ context.Context, workspaceID pgtype.UUID, amount int64, _ string, _ pgtype.UUID) (int64, error) {
	key := pgconv.UUIDString(workspaceID)
	f.balances[key] += amount
	return f.balances[key], nil
}

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

	for k := range body {
		if strings.Contains(strings.ToLower(k), "usd") {
			t.Fatalf("GET /me/credits sent a dollar-denominated field %q to a user: %v", k, body)
		}
	}
	if code, _ := c.postJSON(t, "/admin/credits/"+c.workspaceID+"/grants", `{"amount_credits":10,"reason":"x"}`); code != http.StatusNotFound {
		t.Fatalf("POST grant as a non-operator: got %d, want 404", code)
	}
}

type creditsClient struct {
	*http.Client
	base        string
	userID      string
	workspaceID string
}

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

func TestGrantCreditsIsInvisibleWithoutTheOperatorRole(t *testing.T) {
	pool := creditsTestPool(t)
	_, srv := creditsTestServer(t, pool, newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65}))
	member := creditsLogin(t, srv, "credits-member")

	code, _ := member.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants", `{"amount_credits":10,"reason":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("non-operator grant attempt: got %d, want 404", code)
	}
}

func TestOperatorGrantUnblocksANewSession(t *testing.T) {
	pool := creditsTestPool(t)
	ledger := newFakeCreditLedger(CreditSessionEstimate{
		LowCredits: 30, HighCredits: FallbackSessionThresholdCredits,
		ThresholdCredits: FallbackSessionThresholdCredits, SampleSize: 2, Estimated: true,
	})
	app, srv := creditsTestServer(t, pool, ledger)
	member := creditsLogin(t, srv, "credits-member-grant")
	operator := creditsLogin(t, srv, "credits-operator-grant")

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

func TestAnOperatorGrantCompletesOnOneConnection(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping CRED route test")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	app, err := NewApp(Config{Pool: pool, OAuth: &identity.GitHubOAuth{}, Secure: false, DevLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	h := app.Deps.Credits
	if h == nil || h.Ledger == nil {
		t.Fatal("NewApp wired no credit ledger")
	}
	mux := http.NewServeMux()
	app.Auth.Mount(mux)
	mux.HandleFunc("POST /admin/credits/{workspace_id}/grants", app.Auth.RequireOperator(h.Grant))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	member := creditsLogin(t, srv, "credits-member-oneconn")
	operator := creditsLogin(t, srv, "credits-operator-oneconn")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	operator.Timeout = 10 * time.Second

	code, body := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants", `{"amount_credits":100,"reason":"beta reward"}`)
	if code != http.StatusOK {
		t.Fatalf("operator grant on a one-connection pool: got %d, body %v", code, body)
	}
}

func TestGrantRejectsInvalidRequestsBeforeTheLedgerRuns(t *testing.T) {
	pool := creditsTestPool(t)
	ledger := newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65})
	app, srv := creditsTestServer(t, pool, ledger)
	member := creditsLogin(t, srv, "credits-grant-invalid-member")
	operator := creditsLogin(t, srv, "credits-grant-invalid-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}

	cases := []struct {
		name, path, body string
		want             int
	}{
		{"zero amount", "/admin/credits/" + member.workspaceID + "/grants",
			`{"amount_credits":0,"reason":"x"}`, http.StatusBadRequest},
		{"blank reason", "/admin/credits/" + member.workspaceID + "/grants",
			`{"amount_credits":10,"reason":"   "}`, http.StatusBadRequest},
		{"malformed workspace id", "/admin/credits/not-a-uuid/grants",
			`{"amount_credits":10,"reason":"x"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := operator.postJSON(t, tc.path, tc.body)
			if code != tc.want {
				t.Errorf("got %d, want %d (%v)", code, tc.want, body)
			}
		})
	}

	if len(ledger.balances) != 0 {
		t.Errorf("a rejected grant reached the ledger: balances=%v", ledger.balances)
	}
}

func TestGetCreditsFailsClosedWhenTheBalanceLookupErrors(t *testing.T) {
	pool := creditsTestPool(t)
	ledger := newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65})
	ledger.balanceErr = errors.New("ledger unavailable")
	_, srv := creditsTestServer(t, pool, ledger)
	c := creditsLogin(t, srv, "credits-balance-error")

	code, body := c.getJSON(t, "/me/credits")
	if code != http.StatusInternalServerError {
		t.Fatalf("GET /me/credits with a failing balance lookup: got %d, body %v", code, body)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "balance lookup failed") {
		t.Errorf("the refusal does not name the failing dependency: %v", body)
	}
}

func TestGetCreditsFailsClosedWhenTheSessionEstimateErrors(t *testing.T) {
	pool := creditsTestPool(t)
	ledger := newFakeCreditLedger(CreditSessionEstimate{ThresholdCredits: 65})
	ledger.estimateErr = errors.New("estimate unavailable")
	_, srv := creditsTestServer(t, pool, ledger)
	c := creditsLogin(t, srv, "credits-estimate-error")

	code, body := c.getJSON(t, "/me/credits")
	if code != http.StatusInternalServerError {
		t.Fatalf("GET /me/credits with a failing session estimate: got %d, body %v", code, body)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "session estimate unavailable") {
		t.Errorf("the refusal does not name the failing dependency: %v", body)
	}
}
