package main

import (
	"os"
	"path/filepath"
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
	job := browserWorkflowJob(t, "web-browser-shards")
	for _, environment := range []string{
		"ubuntu-latest\n            browser: chromium",
		"ubuntu-latest\n            browser: firefox",
		"ubuntu-latest\n            browser: webkit",
		"windows-latest\n            browser: chromium",
	} {
		for _, shard := range []string{"1", "2"} {
			row := "- os: " + environment + "\n            shard: " + shard + "\n"
			if strings.Count(job, row) != 1 {
				t.Errorf("expected exactly one matrix entry for %s shard %s", environment, shard)
			}
		}
	}
	if strings.Count(job, "- os:") != 8 {
		t.Error("browser matrix must contain exactly eight shards")
	}
	for _, required := range []string{
		"fail-fast: false",
		"timeout-minutes: 30",
		"npm run test:e2e -- --project=${{ matrix.browser }} --shard=${{ matrix.shard }}/2",
		"name: playwright-report-${{ matrix.os }}-${{ matrix.browser }}-${{ matrix.shard }}-of-2",
	} {
		if !strings.Contains(job, required) {
			t.Errorf("browser shards must retain %q", required)
		}
	}
}

func TestBrowserRequiredChecksFailUnlessEveryShardSucceeded(t *testing.T) {
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

func TestBrowserPreviewPinsOneLoopbackAddress(t *testing.T) {
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
