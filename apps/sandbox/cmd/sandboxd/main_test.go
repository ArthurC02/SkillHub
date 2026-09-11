package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestStartupAdoptsSandboxesBeforeTheResidentProbeCanTearThemDown(t *testing.T) {
	var order []string
	if err := adoptBeforeProtection(func() error {
		order = append(order, "adopt")
		return nil
	}, func() { order = append(order, "protect") }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(order, []string{"adopt", "protect"}) {
		t.Fatalf("startup order = %v, want adopted sandboxes visible before the first probe", order)
	}
	want := errors.New("adoption failed")
	protected := false
	if err := adoptBeforeProtection(func() error { return want }, func() { protected = true }); !errors.Is(err, want) || protected {
		t.Fatalf("failed adoption: err=%v protected=%v", err, protected)
	}
}

func TestRefuseDevSettingsOnAProductionRuntime(t *testing.T) {
	const digest = "ghcr.io/skillhub/runtime-agent-sdk@sha256:" +
		"0000000000000000000000000000000000000000000000000000000000000000"

	if err := refuseDevSettings("runsc", "skillhub/runtime-agent-sdk:2026.08-3", false, false); err == nil {
		t.Error("a tag-pinned image was accepted with runsc: the run record cannot say what ran")
	}
	if err := refuseDevSettings("runsc", digest, true, false); err == nil {
		t.Error("SKILLHUB_SANDBOX_DEV_CMD was accepted with runsc: dev_cmd replaces the harness that enforces the token ceiling")
	}

	err := refuseDevSettings("runsc", digest, false, true)
	if err == nil {
		t.Fatal("SKILLHUB_CLEAN_MODE was accepted with runsc: a runsc node would silently run with no isolation at all")
	}
	if !strings.Contains(err.Error(), "SKILLHUB_CLEAN_MODE") || !strings.Contains(err.Error(), "runsc") {
		t.Errorf("the refusal must name both variables so an operator can find the copied file; got %q", err)
	}

	if err := refuseDevSettings("", "skillhub/runtime-agent-sdk:2026.08-3", true, true); err != nil {
		t.Errorf("a development node was refused: %v", err)
	}
	if err := refuseDevSettings("runsc", digest, false, false); err != nil {
		t.Errorf("a production node with production settings was refused: %v", err)
	}
}

func TestRefuseUnprobedProductionGatesRunscOnAConfiguredProbe(t *testing.T) {
	unconfigured := sandbox.NewP02Probe(nil, 0, 0)
	configured := sandbox.NewP02Probe([]string{"db.internal:5432"}, 0, 0)

	err := refuseUnprobedProduction("runsc", unconfigured)
	if err == nil {
		t.Fatal("runsc with no P-02 targets was accepted: a node with no targets reports not_configured forever")
	}
	if !strings.Contains(err.Error(), "SKILLHUB_SANDBOX_P02_TARGETS") {
		t.Errorf("refusal must name the variable an operator has to set; got %q", err)
	}
	if err := refuseUnprobedProduction("runsc", configured); err != nil {
		t.Errorf("runsc with a configured probe was refused: %v", err)
	}
	if err := refuseUnprobedProduction("", unconfigured); err != nil {
		t.Errorf("a non-runsc node with no targets was refused: %v", err)
	}
}

func TestDriverKindDefaultsToDocker(t *testing.T) {
	if got := driverKind(false); got != "docker" {
		t.Errorf("driverKind(false) = %q, want docker: clean mode unset must not change the driver", got)
	}
	if got := driverKind(true); got != "local" {
		t.Errorf("driverKind(true) = %q, want local", got)
	}
}

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

	empty := t.TempDir()
	if _, err := runnerScriptUnder(empty); err == nil {
		t.Fatal("a repo root with no run.mjs must fail, not return a path that is not there")
	} else if !strings.Contains(err.Error(), empty) {
		t.Fatalf("startup failure must name the path it tried; got %q", err)
	}
}

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

func TestUnenforcedCeilingsMirrorsDetection(t *testing.T) {
	names := resourceLimitNames()
	if len(names) == 0 {
		t.Fatal("resourceLimitNames() read no fields off sandbox.ResourceLimits")
	}

	claims := osCeilings(localdrv.ResourceEnforcement{})
	for _, name := range names {
		if !heldElsewhere[name] {
			if _, ok := claims[name]; !ok {
				t.Errorf("ResourceLimits field %q is classified neither as an OS ceiling "+
					"(osCeilings) nor as held elsewhere (heldElsewhere)", name)
			}
		}
	}

	for name := range claims {
		if heldElsewhere[name] {
			t.Errorf("%q is claimed both as an OS ceiling and as held elsewhere", name)
		}
		if !slices.Contains(names, name) {
			t.Errorf("osCeilings names %q, which sandbox.ResourceLimits does not have", name)
		}
	}

	if got, want := len(claims), reflect.TypeOf(localdrv.ResourceEnforcement{}).NumField(); got != want {
		t.Errorf("osCeilings covers %d ceilings but ResourceEnforcement has %d fields: "+
			"a detection nothing reads is a detection nobody is holding", got, want)
	}

	for _, tc := range []struct {
		name string
		enf  localdrv.ResourceEnforcement
	}{
		{name: "windows job object holds memory and pids only", enf: localdrv.ResourceEnforcement{Memory: true, Processes: true}},
		{name: "unprivileged linux holds nothing", enf: localdrv.ResourceEnforcement{}},
		{name: "a platform that held every ceiling", enf: localdrv.ResourceEnforcement{
			Memory: true, Processes: true, CPU: true, Disk: true, OpenFiles: true,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := osCeilings(tc.enf)
			var want []string
			for _, name := range names {
				if !heldElsewhere[name] && !held[name] {
					want = append(want, name)
				}
			}
			if got := unenforcedCeilings(tc.enf); !slices.Equal(got, want) {
				t.Fatalf("unenforcedCeilings(%+v) = %v, want %v", tc.enf, got, want)
			}
		})
	}

	if got := unenforcedCeilings(localdrv.ResourceEnforcement{}); !slices.Equal(got,
		[]string{"vcpu", "memory_bytes", "disk_bytes", "max_pids", "max_open_files"}) {
		t.Errorf("a platform that enforces nothing declared %v", got)
	}
	if got := unenforcedCeilings(localdrv.ResourceEnforcement{
		Memory: true, Processes: true, CPU: true, Disk: true, OpenFiles: true,
	}); len(got) != 0 {
		t.Errorf("a platform that enforces every ceiling still declared %v unenforced", got)
	}
}
