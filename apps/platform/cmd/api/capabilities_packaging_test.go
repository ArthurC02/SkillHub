package main

// 05 R-36's whole point, applied to the one row that was still missing it.
//
// Measured on 2026-09-02 against a running build: `packaging_download` read
// `unmeasured` with zero profiles loaded (every PACK-001 route answering 503)
// AND with three loaded (packaging working). Identical rows for opposite states,
// so the row carried nothing — while the operator on the demo machine is told
// that /readyz is the answer to "what works here".
//
// The reason it could happen is worth keeping next to the fix: the capability's
// declared precondition, DOWNLOAD_ARTIFACT_RETENTION, is a promise about how
// long an artifact survives. It has nothing to do with whether one can be built.
// A precondition list that names the wrong variable is not a smaller version of
// a probe — it points somewhere else entirely.

import (
	"context"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

func packagingRow(t *testing.T, targets int) envx.Status {
	t.Helper()
	// Every precondition present, so nothing here is measuring a missing variable:
	// the only difference between the two calls below is the profile count.
	env := func(name string) string { return "set" }
	for _, s := range capabilityTable(nil, targets, false).Report(context.Background(), env) {
		if s.ID == "packaging_download" {
			return s
		}
	}
	t.Fatal("the capability table no longer has a packaging_download row")
	return envx.Status{}
}

func TestPackagingReportsBrokenWhenNoTargetsLoaded(t *testing.T) {
	dead := packagingRow(t, 0)
	live := packagingRow(t, 3)

	if dead.Readiness == live.Readiness {
		t.Fatalf("packaging_download reads %q both when zero targets are loaded and "+
			"when three are; a row that is the same whether the capability works or "+
			"is completely dead is the defect 05 R-36 exists to remove", dead.Readiness)
	}
	if dead.Readiness != envx.Broken {
		t.Errorf("with no targets loaded, packaging_download = %q, want %q — a probe ran "+
			"and it failed, which is what broken means", dead.Readiness, envx.Broken)
	}
	if live.Readiness != envx.Ready {
		t.Errorf("with three targets loaded, packaging_download = %q, want %q",
			live.Readiness, envx.Ready)
	}
	// And the broken row has to say what to do about it. The cause is a relative
	// path resolving from the process's working directory, which nobody guesses.
	if !strings.Contains(dead.Detail, "PACKAGING_PROFILES_DIR") {
		t.Errorf("the broken row's detail = %q; it must name the variable whose "+
			"relative default causes this (04 丙-102 ③)", dead.Detail)
	}
}
