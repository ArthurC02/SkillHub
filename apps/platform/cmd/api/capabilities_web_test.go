package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestWebAppDeclaresNoDeploymentVariables(t *testing.T) {
	for _, c := range capabilityTable(nil, 0, true).Capabilities() {
		if c.ID != "web_app" {
			continue
		}
		if len(c.Needs) != 0 {
			t.Fatalf("web_app declares %v; the R-36 checker would then look for them in .env.example", c.Needs)
		}
		return
	}
	t.Fatal("web_app is not in the table")
}

func TestCleanModeDropsOnlyTheWorkerInternalVars(t *testing.T) {
	full := creationCapability(false).Needs
	clean := creationCapability(true).Needs
	dropped := map[string]bool{}
	for _, v := range full {
		dropped[v] = true
	}
	for _, v := range clean {
		delete(dropped, v)
	}
	want := map[string]bool{"CREATION_WORKER_INTERNAL_ADDR": true, "CREATION_WORKER_INTERNAL_URL": true, "CREATION_WORKER_INTERNAL_TOKEN": true}
	if len(dropped) != len(want) {
		t.Fatalf("clean mode dropped %v, want exactly the three Worker internals", dropped)
	}
	for v := range want {
		if !dropped[v] {
			t.Fatalf("clean mode kept %s; it is unreachable in that mode (ADR-060 決策 6)", v)
		}
	}
}
