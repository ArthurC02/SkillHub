package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

var sharedCapabilityIDs = []string{
	"catalogue_search", "intent_search", "evaluation_judge", "run_dispatch",
	"packaging_download", "redistribution_release", "funnel_analytics", "credit_pricing",
	"github_login", "dev_login", "beta_gate", "object_store",
	"generation_entry", "publication_downloads_uninvited",
}

func capabilityIDs(reg *envx.Registry) []string {
	ids := make([]string, 0, len(reg.Capabilities()))
	for _, c := range reg.Capabilities() {
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	return ids
}

func sortedIDs(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

func TestTheProductionCapabilityTableListsEveryRow(t *testing.T) {
	want := sortedIDs(append(slices.Clone(sharedCapabilityIDs), "interactive_creation")...)
	if got := capabilityIDs(capabilityTable(nil, 0)); !slices.Equal(got, want) {
		t.Errorf("production capability rows = %v, want %v", got, want)
	}
}

func TestTheCleanModeCapabilityTableListsEveryRow(t *testing.T) {
	want := sortedIDs(append(slices.Clone(sharedCapabilityIDs), "interactive_creation", "web_app")...)
	if got := capabilityIDs(cleanModeCapabilityTable(nil, 0)); !slices.Equal(got, want) {
		t.Errorf("clean-mode capability rows = %v, want %v", got, want)
	}
}

func TestTheAPIConfigTakesTheSessionPostureAndTheRostersFromTheEnvironment(t *testing.T) {
	t.Setenv("GITHUB_CLIENT_ID", "client-id")
	t.Setenv("OPERATOR_USER_IDS", "op-1, op-2")
	t.Setenv("BETA_ALLOWLIST", "beta-1")
	t.Setenv("CREATION_EXPOSED", "on")
	posture := envx.Posture{SecureCookies: true, AppURL: "https://skillhub.example", DevLogin: false}

	cfg := apiConfigFromEnv(posture)

	if !cfg.Secure || cfg.DevLogin || cfg.AppURL != "https://skillhub.example" {
		t.Errorf("session posture = secure %v, dev login %v, app url %q; want the posture unchanged", cfg.Secure, cfg.DevLogin, cfg.AppURL)
	}
	if !cfg.Operators["op-1"] || !cfg.Operators["op-2"] || len(cfg.Operators) != 2 {
		t.Errorf("operators = %v, want op-1 and op-2", cfg.Operators)
	}
	if !cfg.Invited["beta-1"] || len(cfg.Invited) != 1 {
		t.Errorf("invited = %v, want beta-1", cfg.Invited)
	}
	if !cfg.CreationExposed {
		t.Error("CREATION_EXPOSED=on did not expose creation")
	}
	if cfg.OAuth.ClientID != "client-id" || cfg.OAuth.Client.Timeout != 15*time.Second {
		t.Errorf("oauth = client %q, timeout %v; want client-id and 15s", cfg.OAuth.ClientID, cfg.OAuth.Client.Timeout)
	}
}

func TestTheAnonymousRateLimitAdmitsABurstOfThirtyAndRefusesTheThirtyFirst(t *testing.T) {
	t.Setenv("RATE_LIMIT", "on")
	t.Setenv("TRUSTED_PROXIES", "")
	limiter, err := rateLimitsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	handler := limiter.Limit("characterization", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	status := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=x", nil)
		req.RemoteAddr = "192.0.2.7:4000"
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec.Code
	}
	for i := 1; i <= 30; i++ {
		if got := status(); got != http.StatusOK {
			t.Fatalf("request %d of the burst = %d, want 200", i, got)
		}
	}
	if got := status(); got != http.StatusTooManyRequests {
		t.Errorf("request 31 = %d, want 429", got)
	}
}
