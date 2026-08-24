package main

import "testing"

// The two development settings that must never reach a gVisor node. Both are
// checked in the same place because they fail the same way: the node still
// serves, still injects a Virtual Key, and no longer keeps a promise the
// platform made to the user before the run started.
func TestRefuseDevSettingsOnAProductionRuntime(t *testing.T) {
	const digest = "ghcr.io/skillhub/runtime-agent-sdk@sha256:" +
		"0000000000000000000000000000000000000000000000000000000000000000"

	if err := refuseDevSettings("runsc", "skillhub/runtime-agent-sdk:2026.08-3", false); err == nil {
		t.Error("a tag-pinned image was accepted with runsc: the run record cannot say what ran")
	}
	if err := refuseDevSettings("runsc", digest, true); err == nil {
		t.Error("SKILLHUB_SANDBOX_DEV_CMD was accepted with runsc: dev_cmd replaces the harness that enforces the token ceiling")
	}
	// The dev machine keeps both: it is the runtime, not the variable, that says
	// which node this is.
	if err := refuseDevSettings("", "skillhub/runtime-agent-sdk:2026.08-3", true); err != nil {
		t.Errorf("a development node was refused: %v", err)
	}
	if err := refuseDevSettings("runsc", digest, false); err != nil {
		t.Errorf("a production node with production settings was refused: %v", err)
	}
}
