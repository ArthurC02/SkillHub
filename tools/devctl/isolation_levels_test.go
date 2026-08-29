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
	// Both contracts, each actually read. Half the point of the file list is
	// that a second contract can be added and silently never parsed.
	if len(isolationContractFiles) < 2 {
		t.Fatalf("the contract list is down to %d entries; public.yaml's isolation_level is the "+
			"user-facing half and was the one that drifted", len(isolationContractFiles))
	}
	for _, contract := range isolationContractFiles {
		admitted, err := contractIsolationEnum(filepath.Join(root, filepath.FromSlash(contract.path)), contract.marker)
		if err != nil {
			t.Fatalf("%s: %v", contract.path, err)
		}
		if !admitted["clean"] || !admitted["gvisor"] {
			t.Errorf("%s admits %v; the enum no longer looks like the isolation set, so the "+
				"comparison above passed against something else", contract.path, admitted)
		}
	}
}

// Each contract gets its own negative: a level the gate accepts and that one
// contract does not admit. Without the per-file case, dropping an entry from
// isolationContractFiles would stay green.
func TestIsolationLevelProblemsNamesALevelEachContractDoesNotAdmit(t *testing.T) {
	t.Parallel()
	const gate = `package execution

const (
	productionIsolation = "gvisor"
	cleanIsolation      = "clean"
	// A fourth level added to the gate and nowhere else.
	microvmIsolation = "microvm"
)
`
	full := map[string]string{
		"contracts/openapi/sandbox-provider.yaml": `components:
  schemas:
    ProviderCapability:
      properties:
        isolation:
          properties:
            level:
              type: string
              enum: [gvisor, container, vm, process, clean, microvm]
`,
		"contracts/openapi/public.yaml": `components:
  schemas:
    RunPermissionSummaryContent:
      properties:
        provider:
          properties:
            isolation_level:
              type: string
              enum: [gvisor, container, vm, process, clean, microvm]
`,
	}
	for _, contract := range isolationContractFiles {
		t.Run(contract.path, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeAt(t, root, isolationGoFile, gate)
			for path, body := range full {
				if path == contract.path {
					// Only this one lags behind the gate.
					body = strings.Replace(body, ", microvm]", "]", 1)
				}
				writeAt(t, root, path, body)
			}
			problems := isolationLevelProblems(root)
			if len(problems) != 1 {
				t.Fatalf("want exactly the microvm problem for %s, got %v", contract.path, problems)
			}
			if !strings.Contains(problems[0], "microvm") || !strings.Contains(problems[0], contract.path) {
				t.Fatalf("problem must name the level and the contract that lags; got %q", problems[0])
			}
		})
	}
}

// A free string with the levels listed in a description is what public.yaml
// carried until 331bd90, and it is unreadable to any checker. It must fail, not
// pass by finding no enum to disagree with.
func TestIsolationLevelProblemsRefusesAProseListInsteadOfAnEnum(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, "package execution\n\nconst cleanIsolation = \"clean\"\n")
	writeAt(t, root, "contracts/openapi/sandbox-provider.yaml",
		"        isolation:\n          properties:\n            level:\n              enum: [clean]\n")
	writeAt(t, root, "contracts/openapi/public.yaml",
		"            isolation_level:\n              type: string\n"+
			"              description: 'gvisor | container | vm | process (ADR-015).'\n")
	problems := isolationLevelProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "no enum found under `isolation_level:`") {
		t.Fatalf("a prose list was accepted as a set: %v", problems)
	}
}

// TestIsolationLevelProblemsReadsCodeNotComments is require_db_guard.go's
// lesson applied here: the first version of that checker matched a string that
// also appeared in its own comments, so removing the code left it green.
func TestIsolationLevelProblemsReadsCodeNotComments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, `package execution

// A comment that mentions microvmIsolation = "microvm" and nothing more.
const productionIsolation = "gvisor"
`)
	writeAt(t, root, "contracts/openapi/sandbox-provider.yaml", `        isolation:
          properties:
            level:
              enum: [gvisor, container, vm, process, clean]
`)
	writeAt(t, root, "contracts/openapi/public.yaml", `            isolation_level:
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
