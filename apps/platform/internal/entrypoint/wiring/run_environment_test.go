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
