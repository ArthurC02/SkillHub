package run

import (
	"strings"
	"testing"
)

func TestAProviderWithoutItsTokenIsNamedAndOneWithItIsNot(t *testing.T) {
	t.Setenv("SKILLHUB_SANDBOX_PROVIDERS", "self_hosted=http://127.0.0.1:9000, spare=http://127.0.0.1:9001")
	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED", "a-token")
	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SPARE", "")

	refusals := NewRegistryFromEnv().UnauthenticatedProviderRefusals()
	if len(refusals) != 1 || !strings.Contains(refusals[0], `"spare"`) || !strings.Contains(refusals[0], "SKILLHUB_SANDBOX_TOKEN_SPARE") {
		t.Fatalf("refusals = %q, want only the spare provider named with its variable", refusals)
	}

	t.Setenv("SKILLHUB_SANDBOX_TOKEN_SPARE", "another-token")
	if refusals := NewRegistryFromEnv().UnauthenticatedProviderRefusals(); len(refusals) != 0 {
		t.Fatalf("providers that all carry tokens were refused: %q", refusals)
	}
}
