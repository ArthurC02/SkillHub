package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func browserWorkflowJob(t *testing.T, name string) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, remaining, found := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n  "+name+":\n")
	if !found {
		t.Fatalf("missing browser job %s", name)
	}
	lines := strings.Split(remaining, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			return strings.Join(lines[:index], "\n")
		}
	}
	return remaining
}

func TestBrowserShardsRetainEverySupportedEnvironment(t *testing.T) {
	t.Parallel()
	job := browserWorkflowJob(t, "web-browser-shards")
	for environment, total := range map[string]int{
		"ubuntu-latest\n            browser: chromium":  2,
		"ubuntu-latest\n            browser: firefox":   4,
		"ubuntu-latest\n            browser: webkit":    4,
		"windows-latest\n            browser: chromium": 3,
	} {
		for shard := 1; shard <= total; shard++ {
			row := "- os: " + environment + "\n            shard: " + strconv.Itoa(shard) + "\n            total: " + strconv.Itoa(total) + "\n"
			if strings.Count(job, row) != 1 {
				t.Errorf("expected exactly one matrix entry for %s shard %d of %d", environment, shard, total)
			}
		}
	}
	if strings.Count(job, "- os:") != 13 {
		t.Error("browser matrix must contain exactly thirteen shards")
	}
	for _, required := range []string{
		"fail-fast: false",
		"timeout-minutes: 30",
		"npm run test:e2e -- --project=${{ matrix.browser }} --shard=${{ matrix.shard }}/${{ matrix.total }}",
		"name: playwright-report-${{ matrix.os }}-${{ matrix.browser }}-${{ matrix.shard }}-of-${{ matrix.total }}",
	} {
		if !strings.Contains(job, required) {
			t.Errorf("browser shards must retain %q", required)
		}
	}
}

func TestWebInstallsLeaveAuditingToTheRequiredDependencyAudit(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"web", "web-browser-shards"} {
		job := browserWorkflowJob(t, name)
		if strings.Count(job, "- run: npm ci --no-audit\n") != 2 || strings.Contains(job, "- run: npm ci\n") {
			t.Errorf("%s must install both locked packages without a duplicate advisory request", name)
		}
	}
	job := browserWorkflowJob(t, "dependency-audit")
	for _, command := range []string{
		"'Dependency audit' go -C tools/devctl run . dep-audit\n",
		"'Full dependency audit' go -C tools/devctl run . dep-audit --full\n",
	} {
		if !strings.Contains(job, command) {
			t.Errorf("retain the required audit command %q", command)
		}
	}
	if strings.Contains(job, "continue-on-error:") {
		t.Error("dependency audit failures must fail the job")
	}
}

func TestBrowserRequiredChecksFailUnlessEveryShardSucceeded(t *testing.T) {
	t.Parallel()
	job := browserWorkflowJob(t, "web-browser")
	for _, required := range []string{
		"needs: [changes, web-browser-shards]",
		"if: always() && needs.changes.outputs.web == 'true'",
		"- os: ubuntu-latest\n            browser: chromium",
		"- os: ubuntu-latest\n            browser: firefox",
		"- os: ubuntu-latest\n            browser: webkit",
		"- os: windows-latest\n            browser: chromium",
		"BROWSER_SHARD_RESULT: ${{ needs.web-browser-shards.result }}",
		"test \"$BROWSER_SHARD_RESULT\" = success",
	} {
		if !strings.Contains(job, required) {
			t.Errorf("required browser checks must retain %q", required)
		}
	}
	if strings.Count(job, "- os:") != 4 {
		t.Error("retain exactly the four existing required browser check names")
	}
}

func TestImagePublicationWaitsForNativeBrowserChecks(t *testing.T) {
	t.Parallel()
	job := browserWorkflowJob(t, "images-push")
	_, remaining, found := strings.Cut(job, "    needs: [")
	if !found {
		t.Fatal("image publication must declare its validation dependencies")
	}
	line, _, found := strings.Cut(remaining, "]")
	if !found {
		t.Fatal("image publication dependencies must form a complete list")
	}
	dependencies := make(map[string]bool)
	for _, name := range strings.Split(line, ",") {
		dependencies[strings.TrimSpace(name)] = true
	}
	expected := []string{"images", "web", "web-browser", "platform", "llm", "sandbox", "devctl", "contracts-drift", "dependency-audit"}
	for _, name := range expected {
		if !dependencies[name] {
			t.Errorf("image publication must wait for %s", name)
		}
	}
	if len(dependencies) != len(expected) {
		t.Error("image publication must retain exactly its required validation dependencies")
	}
	if !strings.Contains(job, "if: ${{ !failure() && !cancelled() && needs.images.result == 'success' && github.event_name == 'push' && github.ref == 'refs/heads/main' }}") {
		t.Error("image publication must refuse failed or cancelled validation while allowing unrelated skipped jobs")
	}
}

func TestBrowserPreviewPinsOneLoopbackAddress(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "apps", "web", "playwright.config.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`baseURL: "http://127.0.0.1:4173"`,
		`url: "http://127.0.0.1:4173"`,
		`npm run preview -- --host 127.0.0.1 --port 4173 --strictPort`,
	} {
		if !strings.Contains(string(data), required) {
			t.Errorf("browser and preview server must use the same explicit loopback endpoint: missing %q", required)
		}
	}
}
