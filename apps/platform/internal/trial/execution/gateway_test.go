package run

// The Virtual Key path is a money path and a secret path, so the two rules that
// matter are checked against a stand-in gateway: what goes out on the wire when
// a key is minted, and that revoking a key that is not there is success.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIssueSendsBothBrakesAndScopesTheKey(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"key":"sk-virtual-test"}`))
	}))
	defer srv.Close()

	g := &Gateway{
		AdminBaseURL: srv.URL, adminKey: "sk-master-test", SandboxBaseURL: "http://gateway:4000",
		Model: "gpt-5.4-mini", MaxBudgetUSD: 0.5, TPMLimit: 1000, HTTP: srv.Client(),
	}
	grant, err := g.Issue(context.Background(), "run-1", "attempt-1", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer sk-master-test" {
		t.Errorf("admin key was not used to authenticate: %q", auth)
	}
	// Both brakes, not one: max_budget alone cannot stop a run that spends faster
	// than the gateway flushes (PDM-003 v5).
	if got["max_budget"] != 0.5 {
		t.Errorf("max_budget = %v, want the spend brake", got["max_budget"])
	}
	if got["tpm_limit"] != float64(1000) {
		t.Errorf("tpm_limit = %v, want the rate brake", got["tpm_limit"])
	}
	// A key that can call anything is a budget that means nothing.
	models, _ := got["models"].([]any)
	if len(models) != 1 || models[0] != "gpt-5.4-mini" {
		t.Errorf("models = %v, want the run's own tier only", got["models"])
	}
	if got["key_alias"] != keyAlias("attempt-1") {
		t.Errorf("key_alias = %v, want the attempt-derived alias", got["key_alias"])
	}
	if grant.VirtualKey != "sk-virtual-test" || grant.BaseURL != "http://gateway:4000" {
		t.Errorf("grant = %#v, want the minted key and the sandbox-facing address", grant)
	}
	// The grant must expire on its own even if nothing ever revokes it.
	if grant.ExpiresAt.IsZero() || grant.ExpiresAt.Before(time.Now()) {
		t.Errorf("grant expiry = %v, want a future instant", grant.ExpiresAt)
	}
}

func TestGatewayRefusesAResponsePastItsReadLimit(t *testing.T) {
	const limit = int64(32)
	body := `{"ok":true}` + strings.Repeat(" ", int(limit)-len(`{"ok":true}`)) + "x"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	g := &Gateway{AdminBaseURL: srv.URL, HTTP: srv.Client()}
	var out map[string]any
	if err := g.do(context.Background(), http.MethodGet, "/", nil, &out, limit); err == nil {
		t.Fatal("gateway accepted a response larger than its configured limit")
	}
}

func TestCreationGrantUsesSessionAttribution(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"key":"test-creation-key"}`))
	}))
	defer srv.Close()
	g := &Gateway{AdminBaseURL: srv.URL, HTTP: srv.Client(), Model: "gpt-5.4-mini", MaxBudgetUSD: .1}
	if _, err := g.IssueCreation(context.Background(), "session-1", "receipt-1", 30*time.Second); err != nil {
		t.Fatal(err)
	}
	metadata, _ := got["metadata"].(map[string]any)
	if len(metadata) != 2 || metadata["creation_session_id"] != "session-1" || metadata["creation_attempt_id"] != "receipt-1" {
		t.Fatalf("creation spend attributed to wrong identity: %v", metadata)
	}
	if got["max_budget"] != .1 || got["duration"] != "30s" {
		t.Fatalf("creation grant lost its bound: %v", got)
	}
}

func TestBoundedResponseAcceptsItsExactLimit(t *testing.T) {
	const limit = int64(32)
	body := strings.Repeat("x", int(limit))
	got, err := readBoundedResponse(strings.NewReader(body), limit)
	if err != nil {
		t.Fatalf("exact-limit response refused: %v", err)
	}
	if string(got) != body {
		t.Fatal("exact-limit response changed")
	}
}

func TestGatewayAcceptsOnlyFinalSuccessStatuses(t *testing.T) {
	for _, tc := range []struct {
		code int
		want bool
	}{{101, false}, {199, false}, {200, true}, {299, true}, {300, false}} {
		if got := successfulGatewayStatus(tc.code); got != tc.want {
			t.Errorf("status %d: successfulGatewayStatus = %v, want %v", tc.code, got, tc.want)
		}
	}
}

// A dispatch that cannot mint a key must fail rather than send a sandbox out
// with a route to the gateway and nothing to authenticate with (fail-closed).
func TestIssueFailsWhenTheGatewayAnswersNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	g := &Gateway{AdminBaseURL: srv.URL, adminKey: "k", HTTP: srv.Client()}
	if _, err := g.Issue(context.Background(), "run-1", "attempt-1", time.Minute); err == nil {
		t.Fatal("a gateway that returned no key was treated as success")
	}
}

// Cleanup runs more than once (iron rule 9), and the second pass finds nothing.
func TestRevokeIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Key aliases not found"}}`))
	}))
	defer srv.Close()
	g := &Gateway{AdminBaseURL: srv.URL, adminKey: "k", HTTP: srv.Client()}
	if err := g.Revoke(context.Background(), "attempt-1"); err != nil {
		t.Fatalf("revoking a key that is already gone reported failure: %v", err)
	}
}

// A gateway that is broken must not be read as "revoked": SEC-005 requires the
// key to be gone after a run, and cleanup records the failure so it is retried.
func TestRevokeReportsARealFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	g := &Gateway{AdminBaseURL: srv.URL, adminKey: "k", HTTP: srv.Client()}
	if err := g.Revoke(context.Background(), "attempt-1"); err == nil {
		t.Fatal("a gateway failure was reported as a successful revocation")
	}
}
