package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheRealRepositoryHasNoIsolationDrift is the one that would have caught the
// 2026-08-28 gap, and it runs against the tree rather than a fixture for the
// same reason require_db_guard_test.go does: a checker that only ever sees its
// own fixtures is a checker nobody has pointed at the subject.
func TestTheRealRepositoryHasNoIsolationDrift(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := isolationLevelProblems(root); len(problems) > 0 {
		t.Fatalf("isolation levels drifted: %s", strings.Join(problems, "; "))
	}
	levels, err := gateIsolationLevels(filepath.Join(root, isolationGoFile))
	if err != nil {
		t.Fatal(err)
	}
	// Guard the check's own reach: if the gate's constants were renamed out of
	// the suffix this scans for, the comparison above would pass by finding
	// nothing to compare.
	if len(levels) < 3 {
		t.Fatalf("found %d isolation constants (%v); the gate declares at least gvisor, container and clean, so this check is looking at the wrong thing", len(levels), levels)
	}
}

func TestIsolationLevelProblemsNamesALevelTheContractDoesNotAdmit(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, `package execution

const (
	productionIsolation = "gvisor"
	cleanIsolation      = "clean"
	// A fourth level added to the gate and nowhere else.
	microvmIsolation = "microvm"
)
`)
	writeAt(t, root, isolationContractFile, `components:
  schemas:
    ProviderCapability:
      properties:
        isolation:
          properties:
            level:
              type: string
              enum: [gvisor, container, vm, process, clean]
`)
	problems := isolationLevelProblems(root)
	if len(problems) != 1 {
		t.Fatalf("want exactly the microvm problem, got %v", problems)
	}
	if !strings.Contains(problems[0], "microvm") {
		t.Fatalf("problem must name the level that drifted; got %q", problems[0])
	}
}

// TestIsolationLevelProblemsReadsCodeNotComments is require_db_guard.go's
// lesson applied here: the first version of that checker matched a string that
// also appeared in its own comments, so removing the code left it green.
func TestIsolationLevelProblemsReadsCodeNotComments(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, `package execution

// A comment that mentions microvmIsolation = "microvm" and nothing more.
const productionIsolation = "gvisor"
`)
	writeAt(t, root, isolationContractFile, `        isolation:
          properties:
            level:
              enum: [gvisor, container, vm, process, clean]
`)
	if problems := isolationLevelProblems(root); len(problems) != 0 {
		t.Fatalf("a level that exists only in a comment is not a level; got %v", problems)
	}
}

func writeAt(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
