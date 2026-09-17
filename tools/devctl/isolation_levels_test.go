package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	if len(levels) < 3 {
		t.Fatalf("found %d isolation constants (%v); the gate declares at least strong, weak and none, so this check is looking at the wrong thing", len(levels), levels)
	}

	if len(isolationContractFiles) < 2 {
		t.Fatalf("the contract list is down to %d entries; public.yaml's isolation_strength is the "+
			"user-facing half and was the one that drifted", len(isolationContractFiles))
	}
	for _, contract := range isolationContractFiles {
		admitted, err := contractIsolationEnum(filepath.Join(root, filepath.FromSlash(contract.path)), contract.marker)
		if err != nil {
			t.Fatalf("%s: %v", contract.path, err)
		}
		if !admitted["none"] || !admitted["strong"] {
			t.Errorf("%s admits %v; the enum no longer looks like the isolation set, so the "+
				"comparison above passed against something else", contract.path, admitted)
		}
	}
}

func TestIsolationLevelProblemsNamesALevelEachContractDoesNotAdmit(t *testing.T) {
	t.Parallel()
	const gate = `package execution

const (
	strongIsolation = "strong"
	noIsolation      = "none"
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
              enum: [strong, weak, none, microvm]
`,
		"contracts/openapi/public.yaml": `components:
  schemas:
    RunPermissionSummaryContent:
      properties:
        provider:
          properties:
            isolation_strength:
              type: string
              enum: [strong, weak, none, microvm]
`,
	}
	for _, contract := range isolationContractFiles {
		t.Run(contract.path, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeAt(t, root, isolationGoFile, gate)
			for path, body := range full {
				if path == contract.path {

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

func TestIsolationLevelProblemsRefusesAProseListInsteadOfAnEnum(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, "package execution\n\nconst noIsolation = \"none\"\n")
	writeAt(t, root, "contracts/openapi/sandbox-provider.yaml",
		"        isolation:\n          properties:\n            level:\n              enum: [none]\n")
	writeAt(t, root, "contracts/openapi/public.yaml",
		"            isolation_strength:\n              type: string\n"+
			"              description: 'strong | weak | none.'\n")
	problems := isolationLevelProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "no enum found under `isolation_strength:`") {
		t.Fatalf("a prose list was accepted as a set: %v", problems)
	}
}

func TestIsolationLevelProblemsReadsCodeNotComments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, isolationGoFile, `package execution

// A comment that mentions microvmIsolation = "microvm" and nothing more.
const strongIsolation = "strong"
`)
	writeAt(t, root, "contracts/openapi/sandbox-provider.yaml", `        isolation:
          properties:
            level:
              enum: [strong, weak, none]
`)
	writeAt(t, root, "contracts/openapi/public.yaml", `            isolation_strength:
              enum: [strong, weak, none]
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
