package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformLintPreservesFailureThroughItsLoggingPipeline(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, remaining, found := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"),
		"      - name: golangci-lint (an exclusion that matches nothing fails)\n")
	if !found {
		t.Fatal("platform lint step is missing")
	}
	step, _, _ := strings.Cut(remaining, "\n      -")
	_, script, found := strings.Cut(step, "        run: |\n")
	if !found || strings.TrimSpace(strings.SplitN(script, "\n", 2)[0]) != "set -o pipefail" {
		t.Fatal("platform lint must enable pipefail before the logging pipeline")
	}
}
