package run

import "testing"

// The allow list is rendered, not stringified. While it is empty (SBX-005/006 has
// not minted a grant yet) nothing on screen shows which it is, so this feeds the
// renderer directly rather than waiting for the egress path to open.
//
// It matters twice over: the line is what the user reads before agreeing, and it
// is inside the confirmed hash, so `{model_gateway http://...}` would be both an
// unreadable disclosure and a permanent part of what was agreed to.
func TestEgressAllowIsRenderedAsPurposeAndURL(t *testing.T) {
	lines := egressAllowLines([]egressAllow{
		{Purpose: "model_gateway", URL: "https://gateway.internal/v1"},
		{Purpose: "artifact_upload", URL: "https://objects.internal/put"},
	})
	want := []string{
		"model_gateway: https://gateway.internal/v1",
		"artifact_upload: https://objects.internal/put",
	}
	if len(lines) != len(want) {
		t.Fatalf("rendered %d lines, want %d: %v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	// An empty policy renders an empty list, never a nil that would marshal as
	// `null` and change the hash for a run whose permissions did not change.
	if got := egressAllowLines(nil); got == nil || len(got) != 0 {
		t.Errorf("empty allow list rendered as %#v, want an empty slice", got)
	}
}

// 04 丙-104. The permission summary is the whole of SEC-002's disclosure, and it
// named two secrets unconditionally while a gateway-less deployment injects
// none. Measured on a clean-mode launch: the screen said ANTHROPIC_BASE_URL and
// ANTHROPIC_AUTH_TOKEN would be injected, the driver injected neither, and the
// run went on to report success without ever reaching a model.
//
// The direction matters and is why this is a test: the lie made the run look
// MORE capable than it was, so nothing downstream could contradict it.
func TestInjectedSecretsFollowTheGrantAndNotAConstant(t *testing.T) {
	withGateway := policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "model_gateway", URL: "http://gateway.invalid"}},
	}}
	if got := injectedSecretsFor(withGateway); len(got) != 2 {
		t.Errorf("a run with a gateway grant receives both secrets, got %v", got)
	}

	none := policySnapshot{Egress: EgressPolicy{Mode: "default_deny", Allow: []egressAllow{}}}
	got := injectedSecretsFor(none)
	if len(got) != 0 {
		t.Errorf("no gateway means no secrets are injected, but the summary claims %v", got)
	}
	// Empty, not nil: the row has to render as an empty list rather than vanish.
	if got == nil {
		t.Error("an empty disclosure must still be a list; a missing row reads as a question never asked")
	}

	// A destination that is not the gateway must not carry the secrets either -
	// the grant is what injects them, not the presence of any allowed egress.
	other := policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "something_else", URL: "http://elsewhere.invalid"}},
	}}
	if got := injectedSecretsFor(other); len(got) != 0 {
		t.Errorf("only a model_gateway grant injects these, got %v", got)
	}
}
