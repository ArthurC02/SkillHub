package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The web_app capability (05 R-36, extended to the artifact axis on
// 2026-09-02). Every other mechanism in this repository measures the process
// and its environment; the build the process hands a browser is neither, and
// the gap had a real symptom — a blank page behind a green /readyz.
//
// The probe is driven through probeWebAssetsUnder with a temporary directory,
// the seam webStaticHandlerUnder already uses for the same reason: the failures
// have to be reachable without moving this binary to another machine.

func writeBuild(t *testing.T, index string, assets map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range assets {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWebAssetProbeAcceptsABuildItCanActuallyServe(t *testing.T) {
	dir := writeBuild(t,
		`<script src="/assets/index-abc123.js"></script><link href="/assets/index-abc123.css">`,
		map[string]string{"index-abc123.js": "console.log(1)", "index-abc123.css": "body{}"},
	)
	if err := probeWebAssetsUnder(dir); err != nil {
		t.Fatalf("a complete build was reported broken: %v", err)
	}
}

// The 2026-09-02 failure itself: `task build:web` while the process was up.
// index.html was read into memory at boot and still names the previous hash, so
// every asset request answers 404 and the page renders nothing — while
// /healthz stays 200 and no capability row changes, because none of them looks
// at the build.
func TestWebAssetProbeCatchesARebuildWithoutARestart(t *testing.T) {
	dir := writeBuild(t,
		`<script src="/assets/index-OLDHASH.js"></script>`,
		map[string]string{"index-NEWHASH.js": "console.log(1)"},
	)
	err := probeWebAssetsUnder(dir)
	if err == nil {
		t.Fatal("a stale index.html pointing at a hash that no longer exists was reported healthy")
	}
	if !strings.Contains(err.Error(), "index-OLDHASH.js") {
		t.Errorf("the error must name the missing file so an operator can act on it; got %v", err)
	}
}

func TestWebAssetProbeRefusesAnEmptyAsset(t *testing.T) {
	dir := writeBuild(t,
		`<script src="/assets/index-abc123.js"></script>`,
		map[string]string{"index-abc123.js": ""},
	)
	if err := probeWebAssetsUnder(dir); err == nil {
		t.Fatal("a zero-byte bundle was reported healthy")
	}
}

// An index.html that references nothing is not a production build — the dev
// server's entry point is `/src/main.tsx`. Reporting it Ready would mean the
// probe passes hardest on the one input that serves no application at all.
func TestWebAssetProbeRefusesAnIndexThatReferencesNoBuildOutput(t *testing.T) {
	dir := writeBuild(t, `<script type="module" src="/src/main.tsx"></script>`, nil)
	if err := probeWebAssetsUnder(dir); err == nil {
		t.Fatal("an index.html with no built assets was reported healthy")
	}
}

func TestWebAssetProbeSaysSoWhenThereIsNoBuildAtAll(t *testing.T) {
	if err := probeWebAssetsUnder(filepath.Join(t.TempDir(), "never-built")); err == nil {
		t.Fatal("a missing build directory was reported healthy")
	}
}

// The row exists only where this process serves the SPA. Elsewhere the build is
// behind something else (ADR-018 E1) and both Unavailable and Broken would be
// this deployment answering for somebody else's.
func TestWebAppRowIsDeclaredOnlyWhereThisProcessServesTheBuild(t *testing.T) {
	has := func(servesWeb bool) bool {
		for _, c := range capabilityTable(nil, 0, servesWeb).Capabilities() {
			if c.ID == "web_app" {
				return true
			}
		}
		return false
	}
	if has(false) {
		t.Error("web_app was declared by a deployment that serves no build")
	}
	if !has(true) {
		t.Error("web_app was missing from a deployment that serves the build")
	}
}

// R-36's rule, applied to this row: Ready is reachable only by measurement.
// A row whose probe is nil would report Unmeasured forever and teach nothing.
func TestWebAppRowCarriesAProbe(t *testing.T) {
	for _, c := range capabilityTable(nil, 0, true).Capabilities() {
		if c.ID != "web_app" {
			continue
		}
		if c.Probe == nil {
			t.Fatal("web_app has no probe, so it can never be anything but unmeasured")
		}
		if c.Without == "" || c.Fix == "" {
			t.Fatal("a row without 沒有它會怎樣／怎麼補 is the shape R-36 exists to remove")
		}
		return
	}
	t.Fatal("web_app is not in the table")
}

// The declared table the R-36 checker reads must stay free of variables this
// row does not have: web_app is gated by an artifact, not by configuration, and
// a Needs entry here would send the checker looking for it in .env.example.
func TestWebAppDeclaresNoDeploymentVariables(t *testing.T) {
	before := capabilityTable(nil, 0, false).DeclaredVars()
	after := capabilityTable(nil, 0, true).DeclaredVars()
	if len(before) != len(after) {
		t.Fatalf("web_app changed the declared variable set: %v -> %v", before, after)
	}
}
