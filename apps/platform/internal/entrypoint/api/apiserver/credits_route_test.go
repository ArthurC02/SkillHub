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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
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

func (f *fakeCreditLedger) Standing(_ context.Context, workspaceID pgtype.UUID) (int64, bool, error) {
	if f.balanceErr != nil {
		return 0, false, f.balanceErr
	}
	balance := f.balances[pgconv.UUIDString(workspaceID)]
	return balance, balance >= f.estimate.ThresholdCredits, nil
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
		LowCredits: 30, HighCredits: testSessionThreshold,
		ThresholdCredits: testSessionThreshold, SampleSize: 2, Estimated: true,
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
		t.Fatalf("a fresh account (balance 0) reported can_start = true against threshold %d", testSessionThreshold)
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
		t.Fatalf("balance 100 >= threshold %d but can_start is still false: %v", testSessionThreshold, after)
	}
	if reason, _ := after["block_reason"].(string); reason != "" {
		t.Fatalf("can_start but block_reason is still %q", reason)
	}
}

func creditsOneConnectionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	return pool
}

func realCreditsServer(t *testing.T, pool *pgxpool.Pool) (*App, *httptest.Server) {
	t.Helper()
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
	mux.HandleFunc("GET /me/credits", app.Auth.RequireSession(h.Get))
	mux.HandleFunc("POST /admin/credits/{workspace_id}/grants", app.Auth.RequireOperator(h.Grant))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return app, srv
}

func TestTheBalanceScreenAsksTheLedgerWhetherASessionMayStart(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-standing-member")
	operator := creditsLogin(t, srv, "credits-standing-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}

	code, before := member.getJSON(t, "/me/credits")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits: got %d, body %v", code, before)
	}
	if canStart, _ := before["can_start"].(bool); canStart {
		t.Fatalf("a fresh account with 0 credits was told it can start a session: %v", before)
	}

	if code, body := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants",
		`{"amount_credits":1000000,"reason":"beta reward"}`); code != http.StatusOK {
		t.Fatalf("grant: got %d, body %v", code, body)
	}
	code, after := member.getJSON(t, "/me/credits")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits after the grant: got %d, body %v", code, after)
	}
	if canStart, _ := after["can_start"].(bool); !canStart {
		t.Fatalf("1,000,000 credits and the screen still says a session cannot start: %v", after)
	}
}

func TestAnOperatorGrantCompletesOnOneConnection(t *testing.T) {
	app, srv := realCreditsServer(t, creditsOneConnectionPool(t))
	member := creditsLogin(t, srv, "credits-member-oneconn")
	operator := creditsLogin(t, srv, "credits-operator-oneconn")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	operator.Timeout = 10 * time.Second

	code, body := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants", `{"amount_credits":100,"reason":"beta reward"}`)
	if code != http.StatusOK {
		t.Fatalf("operator grant on a one-connection pool: got %d, body %v", code, body)
	}
}

func TestAnOperatorCorrectionLowersTheBalance(t *testing.T) {
	pool := creditsTestPool(t)
	app, srv := realCreditsServer(t, pool)
	member := creditsLogin(t, srv, "credits-member-correction")
	operator := creditsLogin(t, srv, "credits-operator-correction")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"

	code, granted := operator.postJSON(t, grants, `{"amount_credits":100,"reason":"beta reward"}`)
	if code != http.StatusOK {
		t.Fatalf("grant: got %d, body %v", code, granted)
	}
	before, _ := granted["balance_credits"].(float64)

	code, corrected := operator.postJSON(t, grants, `{"amount_credits":-30,"reason":"corrects an over-grant"}`)
	if code != http.StatusOK {
		t.Fatalf("a corrective adjustment the contract allows: got %d, body %v", code, corrected)
	}
	if corrected["balance_credits"] != before-30 {
		t.Errorf("balance after the correction = %v, want %v", corrected["balance_credits"], before-30)
	}
	var kind string
	if err := pool.QueryRow(context.Background(), `
		SELECT kind FROM credit_entries WHERE user_id = $1 AND delta_credits = -30`,
		mustParseUUID(t, member.userID)).Scan(&kind); err != nil {
		t.Fatalf("reading the correction's ledger entry: %v", err)
	}
	if kind != "adjustment" {
		t.Errorf("a negative grant was booked as %q, want adjustment", kind)
	}
}

func TestGrantingToAWorkspaceThatDoesNotExistIsNotFound(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	operator := creditsLogin(t, srv, "credits-operator-unknown-workspace")
	app.Auth.Operators = map[string]bool{operator.userID: true}

	code, body := operator.postJSON(t, "/admin/credits/"+uuid.NewString()+"/grants", `{"amount_credits":10,"reason":"beta reward"}`)
	if code != http.StatusNotFound {
		t.Fatalf("grant to a workspace that does not exist: got %d (%v), want 404", code, body)
	}
}

func TestCreationSettlementCompletesOnOneConnection(t *testing.T) {
	_, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-settle-oneconn")
	workspaceID := mustParseUUID(t, member.workspaceID)

	pool := creditsOneConnectionPool(t)
	svc, err := wiring.NewCreditService(pool)
	if err != nil {
		t.Fatal(err)
	}
	target := &creation.Service{}
	wiring.WireCreationCredit(target, svc, pool)

	for _, tc := range []struct {
		name      string
		usdMicros int64
	}{
		{"a paid step", 3_000},
		{"a step that cost nothing", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			spent := tc.usdMicros
			if err := target.CreditSettle(ctx, tx, workspaceID, mustParseUUID(t, uuid.NewString()), 1, &spent, 100_000); err != nil {
				t.Fatalf("settling a creation step on one connection: %v", err)
			}
		})
	}
}

func mustParseUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return id
}

func TestGrantRefusesAZeroAmountOrABlankReasonAndWritesNoEntry(t *testing.T) {
	pool := creditsTestPool(t)
	app, srv := realCreditsServer(t, pool)
	member := creditsLogin(t, srv, "credits-grant-invalid-member")
	operator := creditsLogin(t, srv, "credits-grant-invalid-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"

	cases := []struct {
		name, path, body, message string
		want                      int
	}{
		{"zero amount", grants, `{"amount_credits":0,"reason":"x"}`,
			"amount_credits must not be zero", http.StatusBadRequest},
		{"blank reason", grants, `{"amount_credits":10,"reason":"   "}`,
			"reason is required", http.StatusBadRequest},
		{"malformed workspace id", "/admin/credits/not-a-uuid/grants", `{"amount_credits":10,"reason":"x"}`,
			"workspace not found", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := operator.postJSON(t, tc.path, tc.body)
			if code != tc.want {
				t.Fatalf("got %d, want %d (%v)", code, tc.want, body)
			}
			if msg, _ := body["error"].(string); msg != tc.message {
				t.Errorf("error = %q, want %q", msg, tc.message)
			}
		})
	}

	var entries int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM credit_entries WHERE user_id = $1`, mustParseUUID(t, member.userID)).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if entries != 0 {
		t.Errorf("a refused grant wrote %d ledger entries", entries)
	}
}

func TestASuccessfulGrantIsAuditedWithItsTargetWorkspace(t *testing.T) {
	pool := creditsTestPool(t)
	app, srv := realCreditsServer(t, pool)
	member := creditsLogin(t, srv, "credits-grant-audit-member")
	operator := creditsLogin(t, srv, "credits-grant-audit-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}

	code, body := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants",
		`{"amount_credits":40,"reason":"  beta reward  "}`)
	if code != http.StatusOK {
		t.Fatalf("grant: got %d, body %v", code, body)
	}

	var workspaceID, reason string
	var credits int64
	if err := pool.QueryRow(context.Background(), `
		SELECT workspace_id::text, metadata->>'reason', (metadata->>'credits')::bigint
		FROM audit_events
		WHERE action = 'credit.grant' AND actor_user_id = $1`,
		mustParseUUID(t, operator.userID)).Scan(&workspaceID, &reason, &credits); err != nil {
		t.Fatalf("reading the grant's audit event: %v", err)
	}
	if workspaceID != member.workspaceID {
		t.Errorf("audit workspace = %q, want the granted workspace %q", workspaceID, member.workspaceID)
	}
	if reason != "beta reward" || credits != 40 {
		t.Errorf("audit metadata reason=%q credits=%d, want %q and 40", reason, credits, "beta reward")
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

func (f *fakeCreditLedger) Ledger(context.Context, pgtype.UUID, pgtype.UUID) (credit.Ledger, error) {
	return credit.Ledger{}, nil
}

func (f *fakeCreditLedger) CostStatistics(context.Context) ([]credit.KindStatistics, error) {
	return nil, nil
}

func (f *fakeCreditLedger) DailyCost(context.Context, time.Time) ([]credit.DailyAmount, error) {
	return nil, nil
}

func (f *fakeCreditLedger) DailyCredits(context.Context, time.Time) ([]credit.DailyAmount, int64, error) {
	return nil, 0, nil
}
