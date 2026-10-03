package wiring

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewRunRegistryFromEnvNamesOnlyProvidersWithoutTokens(t *testing.T) {
	t.Setenv("SKILLHUB_SANDBOX_PROVIDERS", "self_hosted=http://127.0.0.1:9000, spare=http://127.0.0.1:9001")
	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED", "a-token")
	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SPARE", "")

	refusals := NewRunRegistryFromEnv().UnauthenticatedProviderRefusals()
	if len(refusals) != 1 || !strings.Contains(refusals[0], `"spare"`) || !strings.Contains(refusals[0], "SKILLHUB_SANDBOX_TOKEN_SPARE") {
		t.Fatalf("refusals = %q, want only the spare provider named with its variable", refusals)
	}

	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SPARE", "another-token")
	if refusals := NewRunRegistryFromEnv().UnauthenticatedProviderRefusals(); len(refusals) != 0 {
		t.Fatalf("providers that all carry tokens were refused: %q", refusals)
	}
}

func TestRunDeploymentFromEnvPicksTheIsolationFloorFromTheDeploymentMode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cleanMode    string
		devLogin     string
		wantIsolated string
		wantClean    bool
	}{
		{"production", "", "", "strong", false},
		{"dev login", "", "1", "weak", false},
		{"clean mode", "1", "", "none", true},
		{"clean mode wins over dev login", "1", "1", "none", true},
		{"clean mode flag other than 1 is off", "true", "", "strong", false},
		{"dev login flag other than 1 is off", "", "true", "strong", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SKILLHUB_CLEAN_MODE", tc.cleanMode)
			t.Setenv("DEV_LOGIN", tc.devLogin)
			t.Setenv("SKILLHUB_CLEAN_MODE_RELEASES", "/tmp/releases.txt")

			got := RunDeploymentFromEnv()

			if string(got.MinimumIsolation) != tc.wantIsolated || got.CleanMode != tc.wantClean {
				t.Errorf("isolation=%q clean=%v, want %q and %v", got.MinimumIsolation, got.CleanMode, tc.wantIsolated, tc.wantClean)
			}
			if got.CleanModeReleases != "/tmp/releases.txt" {
				t.Errorf("CleanModeReleases = %q, want the path from SKILLHUB_CLEAN_MODE_RELEASES", got.CleanModeReleases)
			}
		})
	}
}

func TestOnlyARunBudgetThatIsSetButUnusableIsReported(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		reported bool
	}{
		{"", false},
		{"0.75", false},
		{"0", true},
		{"0.5usd", true},
		{"NaN", true},
		{"Inf", true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			t.Setenv("SKILLHUB_RUN_MAX_BUDGET_USD", tc.raw)

			runBudgetFromEnv()

			if got := strings.Contains(logged.String(), "SKILLHUB_RUN_MAX_BUDGET_USD"); got != tc.reported {
				t.Errorf("reported = %v, want %v: %q", got, tc.reported, logged.String())
			}
		})
	}
}

func TestOnlyATPMLimitThatIsSetButUnusableIsReported(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		reported bool
	}{
		{"", false},
		{"200000", false},
		{"0", true},
		{"50k", true},
		{"200_000", true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			t.Setenv("SKILLHUB_RUN_TPM_LIMIT", tc.raw)

			runTPMLimitFromEnv()

			if got := strings.Contains(logged.String(), "SKILLHUB_RUN_TPM_LIMIT"); got != tc.reported {
				t.Errorf("reported = %v, want %v: %q", got, tc.reported, logged.String())
			}
		})
	}
}
