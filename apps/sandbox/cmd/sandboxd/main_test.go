package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
)

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

// TestDriverKindDefaultsToDocker is the guard for 02:PORT-005 and ADR-060
// decision 6.4: with SKILLHUB_CLEAN_MODE unset (cleanMode=false), this process
// must wire up the exact driver it did before this axis existed. Flip the
// false branch to also return "local" and this goes red - that is the
// mutation ADR-060's task asks to actually run, not just claim.
func TestDriverKindDefaultsToDocker(t *testing.T) {
	if got := driverKind(false); got != "docker" {
		t.Errorf("driverKind(false) = %q, want docker: clean mode unset must not change the driver", got)
	}
	if got := driverKind(true); got != "local" {
		t.Errorf("driverKind(true) = %q, want local", got)
	}
}

// TestResolveIsolationDeclaresCleanHonestly guards ADR-059 decision 1: clean
// mode must never be declared as the stronger gvisor or container level. That
// is the second mutation the task asks to run - swap "clean" for "gvisor" in
// resolveIsolation and this test must go red.
func TestResolveIsolationDeclaresCleanHonestly(t *testing.T) {
	cases := []struct {
		name      string
		cleanMode bool
		runtime   string
		want      string
	}{
		{"clean mode wins even over runsc", true, "runsc", "clean"},
		{"clean mode, no runtime", true, "", "clean"},
		{"runsc without clean mode is gvisor", false, "runsc", "gvisor"},
		{"neither is the dev container level", false, "", "container"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveIsolation(c.cleanMode, c.runtime); got != c.want {
				t.Errorf("resolveIsolation(%v, %q) = %q, want %q", c.cleanMode, c.runtime, got, c.want)
			}
		})
	}
}

// TestCleanModeRunnerScriptFindsRunMjs guards the path resolution that stands
// in for a new env var (ADR-060 decision 6.2 forbids one per axis): it must
// resolve to the real run.mjs regardless of the test's working directory.
func TestCleanModeRunnerScriptFindsRunMjs(t *testing.T) {
	got, err := cleanModeRunnerScript()
	if err != nil {
		t.Fatalf("cleanModeRunnerScript(): %v", err)
	}
	if filepath.Base(got) != "run.mjs" {
		t.Fatalf("cleanModeRunnerScript() = %q, want a path ending in run.mjs", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("cleanModeRunnerScript() = %q does not exist: %v", got, err)
	}
	// The path is derived from the build machine's source layout, so "missing"
	// is a reachable state, and 02:PORT-005 requires the startup failure to name
	// what is missing. Drive the real function with a root that has no script.
	empty := t.TempDir()
	if _, err := runnerScriptUnder(empty); err == nil {
		t.Fatal("a repo root with no run.mjs must fail, not return a path that is not there")
	} else if !strings.Contains(err.Error(), empty) {
		t.Fatalf("startup failure must name the path it tried; got %q", err)
	}
}

// TestCleanModeMaxResourcesReflectsDetection is 02:PORT-010's literal
// requirement in code: the declared ceiling must not silently claim
// enforcement the driver did not detect. sandbox.Config.accept() (outside
// this file's allowlist) requires every ceiling to be > 0, so this still
// returns DefaultLimits' numbers either way - the honesty this function adds
// is the log warning, not a different number. See cleanModeMaxResources's own
// doc comment for the trade-off this asserts.
func TestCleanModeMaxResourcesReflectsDetection(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	got := cleanModeMaxResources(localdrv.ResourceEnforcement{Memory: true, Processes: true}, log)
	if got.MemoryBytes <= 0 || got.MaxPIDs <= 0 {
		t.Fatalf("cleanModeMaxResources with full enforcement returned a non-positive ceiling: %+v", got)
	}

	notEnforced := cleanModeMaxResources(localdrv.ResourceEnforcement{Memory: false, Processes: false}, log)
	if notEnforced.MemoryBytes <= 0 || notEnforced.MaxPIDs <= 0 {
		t.Fatalf("cleanModeMaxResources with no enforcement returned a non-positive ceiling: %+v; "+
			"sandbox.Config.accept() requires every field > 0, so this must stay positive", notEnforced)
	}
	if notEnforced != got {
		t.Errorf("cleanModeMaxResources changed the declared numbers based on enforcement (%+v vs %+v); "+
			"the contract has no field for \"declared but unenforced\", so the two must match "+
			"and the only honest signal is the log warning", notEnforced, got)
	}
}
