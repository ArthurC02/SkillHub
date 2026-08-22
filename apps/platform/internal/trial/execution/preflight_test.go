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
