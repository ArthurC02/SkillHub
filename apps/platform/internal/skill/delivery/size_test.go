package packaging

// 03:PACK-012 and its half of 03:INGEST-016: the produced ceiling is a different
// question from the import ceiling, and the refusal is counted.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

// The defect PACK-012 names is not "the number is wrong", it is "there is one
// number for two questions". Packaging always adds INSTALL.md, the manifest and
// any test cases that travel, so produced > source by construction: sharing the
// import ceiling means a package that imported at the ceiling, ran, and was
// evaluated is then refused at the last step, with all the work already done.
//
// That is true at ANY value of the import ceiling, which is why this does not
// wait on 05 R-13.
func TestAPackageJustOverTheImportCeilingStillPackages(t *testing.T) {
	if err := checkProducedSize(skillpkg.MaxZipBytes + 1); err != nil {
		t.Fatalf("a produced package one byte over the IMPORT ceiling was refused, "+
			"which is the dead zone PACK-012 exists to close: %v", err)
	}
	if err := checkProducedSize(MaxProducedZipBytes); err != nil {
		t.Errorf("a produced package exactly at its own ceiling was refused: %v", err)
	}
	err := checkProducedSize(MaxProducedZipBytes + 1)
	if err == nil {
		t.Fatal("a produced package over the produced ceiling was accepted")
	}
	// The refusal names both numbers, for the same reason an upload refusal does.
	if !strings.Contains(err.Error(), "over the") || !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("the produced refusal does not name its ceiling: %v", err)
	}
}

// The headroom has to be more than a rounding gesture: the two files the platform
// always adds, plus at least one test case at the text ceilings PDM-005 §5.1 puts
// on it, have to fit. Anything less and the decoupling is nominal.
func TestTheProducedHeadroomFitsWhatPackagingCanAdd(t *testing.T) {
	headroom := MaxProducedZipBytes - skillpkg.MaxZipBytes
	if headroom <= 0 {
		t.Fatalf("the produced ceiling (%d) is not above the import ceiling (%d)",
			MaxProducedZipBytes, skillpkg.MaxZipBytes)
	}
	var largestInstall int
	for _, p := range loadRealProfiles(t).Ordered() {
		if n := len(renderInstall(p, "demo-skill", nil)); n > largestInstall {
			largestInstall = n
		}
	}
	// One case.json at its ceiling: name + prompt + every criterion at full length.
	perCase := testlab.MaxNameBytes + testlab.MaxPromptBytes +
		testlab.MaxCriteria*testlab.MaxCriterionBytes
	if headroom < largestInstall+perCase {
		t.Errorf("headroom is %d bytes; INSTALL.md alone is up to %d and one test case at "+
			"PDM-005 §5.1's text ceiling is %d", headroom, largestInstall, perCase)
	}
}

// 03:INGEST-016's counting half, on the path that is ours rather than a user's.
func TestAProducedRefusalIsCounted(t *testing.T) {
	before := refusalCount(t, metrics.CeilingProduced)
	if err := checkProducedSize(MaxProducedZipBytes + 1); err == nil {
		t.Fatal("want a refusal")
	}
	if got := refusalCount(t, metrics.CeilingProduced) - before; got != 1 {
		t.Errorf("produced-ceiling refusals counted %v times, want 1", got)
	}
	// A package that fits must not move it — a counter that also counts successes
	// answers "is a ceiling too tight" with noise (05 R-13's safety net).
	if err := checkProducedSize(skillpkg.MaxZipBytes + 1); err != nil {
		t.Fatal(err)
	}
	if got := refusalCount(t, metrics.CeilingProduced) - before; got != 1 {
		t.Errorf("an accepted package moved the refusal counter to %v", got)
	}
}

// The three ceilings have to stay tellable apart. They are the three doors a
// package can die at and the right response to each is different: `upload` says
// the ceiling may be too tight for creators, `url` says an external source is,
// and `produced` says our own packager is over its own ceiling.
func TestTheThreeSizeCeilingsAreDistinctLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range []string{metrics.CeilingUpload, metrics.CeilingURL, metrics.CeilingProduced} {
		if l == "" || seen[l] {
			t.Fatalf("size ceiling labels are not three distinct values: %q repeats or is empty", l)
		}
		seen[l] = true
	}
}

// refusalCount reads one ceiling's counter off the real exposition rather than
// out of the collector, so the assertion also covers the half that matters to an
// operator: the series is registered and it is actually exported. A number only
// the process can see is the state 03:INGEST-016 exists to leave.
func refusalCount(t *testing.T, ceiling string) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	prefix := `skillhub_package_size_refused_total{ceiling="` + ceiling + `"} `
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				t.Fatalf("unparseable counter line %q: %v", line, err)
			}
			return v
		}
	}
	return 0 // never incremented: Prometheus does not export an untouched child
}

// The other half of PACK-012: decoupling makes a produced package that this
// platform will not take back possible, and that is a fact a reader can walk
// into. INSTALL.md is where it is said, because it is the only surface that
// travels with the bytes.
func TestInstallInstructionsNameTheImportCeilingSoTheRoundTripIsNotAssumed(t *testing.T) {
	for _, p := range loadRealProfiles(t).Ordered() {
		out := renderInstall(p, "demo-skill", nil)
		if !strings.Contains(out, skillpkg.HumanMB(skillpkg.MaxZipBytes)) {
			t.Errorf("%s: INSTALL.md does not name the import ceiling:\n%s", p.ID, out)
		}
		if !strings.Contains(out, "Skill Hub will not take it back") {
			t.Errorf("%s: INSTALL.md names a number without saying what happens at it:\n%s", p.ID, out)
		}
	}
}
