package packaging

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

	if !strings.Contains(err.Error(), "over the") || !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("the produced refusal does not name its ceiling: %v", err)
	}
}

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

	perCase := testlab.MaxNameBytes + testlab.MaxPromptBytes +
		testlab.MaxCriteria*testlab.MaxCriterionBytes
	if headroom < largestInstall+perCase {
		t.Errorf("headroom is %d bytes; INSTALL.md alone is up to %d and one test case at "+
			"PDM-005 §5.1's text ceiling is %d", headroom, largestInstall, perCase)
	}
}

func TestAProducedRefusalIsCounted(t *testing.T) {
	before := refusalCount(t, metrics.CeilingProduced)
	if err := checkProducedSize(MaxProducedZipBytes + 1); err == nil {
		t.Fatal("want a refusal")
	}
	if got := refusalCount(t, metrics.CeilingProduced) - before; got != 1 {
		t.Errorf("produced-ceiling refusals counted %v times, want 1", got)
	}

	if err := checkProducedSize(skillpkg.MaxZipBytes + 1); err != nil {
		t.Fatal(err)
	}
	if got := refusalCount(t, metrics.CeilingProduced) - before; got != 1 {
		t.Errorf("an accepted package moved the refusal counter to %v", got)
	}
}

func TestTheThreeSizeCeilingsAreDistinctLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range []string{metrics.CeilingUpload, metrics.CeilingURL, metrics.CeilingProduced} {
		if l == "" || seen[l] {
			t.Fatalf("size ceiling labels are not three distinct values: %q repeats or is empty", l)
		}
		seen[l] = true
	}
}

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
	return 0
}

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
