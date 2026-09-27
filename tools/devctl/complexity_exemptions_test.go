package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func complexityFixtureLint(t *testing.T, enableGocognit bool, minComplexity string, exemptionCount int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: \"2\"\n\nlinters:\n  enable:\n")
	if enableGocognit {
		b.WriteString("    - gocognit\n")
	} else {
		b.WriteString("    - depguard\n")
	}
	b.WriteString("\n  exclusions:\n    presets:\n      - std-error-handling\n    warn-unused: true\n    rules:\n")
	b.WriteString("      - path: _test\\.go\n        linters: [gocognit]\n")
	for i := 0; i < exemptionCount; i++ {
		fmt.Fprintf(&b, "      - path: fixture%d.go\n        linters: [gocognit]\n        text: 'func `Fn%d`'\n", i, i)
	}
	b.WriteString("\n  settings:\n    gocognit:\n")
	if minComplexity != "" {
		fmt.Fprintf(&b, "      min-complexity: %s\n", minComplexity)
	}
	return b.String()
}

func withComplexityFixture(t *testing.T, registeredExempt int, contents string) string {
	t.Helper()
	root := t.TempDir()
	const lintPath = "fixture/.golangci.yml"
	writeAt(t, root, lintPath, contents)

	original := complexityGateModules
	complexityGateModules = []complexityGateModule{{lintPath: lintPath, registeredExempt: registeredExempt}}
	t.Cleanup(func() { complexityGateModules = original })
	return root
}

func TestComplexityExemptionsAcceptsAnExactMatch(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, true, "30", 2))
	if problems := complexityExemptionProblems(root); len(problems) != 0 {
		t.Fatalf("a module whose exemption count matches the registry was rejected: %v", problems)
	}
}

func TestComplexityExemptionsRejectsOneMoreExemptionThanRegistered(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, true, "30", 3))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "carries 3 per-function gocognit exemptions, more than the registered 2") {
		t.Fatalf("an over-registered exemption count was accepted: %v", problems)
	}
}

func TestComplexityExemptionsRejectsOneFewerExemptionThanRegistered(t *testing.T) {
	root := withComplexityFixture(t, 3, complexityFixtureLint(t, true, "30", 2))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "carries 2 per-function gocognit exemptions, fewer than the registered 3") {
		t.Fatalf("an under-registered exemption count was accepted: %v", problems)
	}
}

func TestComplexityExemptionsRejectsGocognitNotEnabled(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, false, "30", 2))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "does not enable gocognit") {
		t.Fatalf("a module that never enables gocognit was accepted: %v", problems)
	}
}

func TestComplexityExemptionsRejectsAMissingMinComplexity(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, true, "", 2))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "sets no gocognit min-complexity") {
		t.Fatalf("a module with no min-complexity setting was accepted: %v", problems)
	}
}

func TestComplexityExemptionsAcceptsTheCeilingBoundary(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, true, "30", 2))
	if problems := complexityExemptionProblems(root); len(problems) != 0 {
		t.Fatalf("min-complexity at the registered ceiling was rejected: %v", problems)
	}
}

func TestComplexityExemptionsRejectsOneOverTheCeilingBoundary(t *testing.T) {
	root := withComplexityFixture(t, 2, complexityFixtureLint(t, true, "31", 2))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "sets min-complexity 31, above the registered ceiling 30") {
		t.Fatalf("a raised gocognit ceiling was accepted: %v", problems)
	}
}

func TestComplexityExemptionsReportsAMissingLintFile(t *testing.T) {
	root := t.TempDir()
	original := complexityGateModules
	complexityGateModules = []complexityGateModule{{lintPath: "does/not/exist.yml", registeredExempt: 0}}
	t.Cleanup(func() { complexityGateModules = original })

	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "does/not/exist.yml") {
		t.Fatalf("a missing .golangci.yml was not reported by its own path: %v", problems)
	}
}

func TestComplexityExemptionsOnTheRealRepoConfigsHasNoProblems(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := complexityExemptionProblems(root); len(problems) != 0 {
		t.Fatalf("the repo's three .golangci.yml files disagree with the registered exemption counts: %v", problems)
	}
}

func TestComplexityExemptionsCheckerHasNoUnusedFixtureFiles(t *testing.T) {
	// Sanity check on the fixture builder itself: an empty exemption count
	// must not accidentally match a nonzero file glob (T9, false green).
	root := t.TempDir()
	path := filepath.Join(root, "fixture", ".golangci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(complexityFixtureLint(t, true, "30", 0)), 0o600); err != nil {
		t.Fatal(err)
	}
	original := complexityGateModules
	complexityGateModules = []complexityGateModule{{lintPath: "fixture/.golangci.yml", registeredExempt: 0}}
	t.Cleanup(func() { complexityGateModules = original })

	if problems := complexityExemptionProblems(root); len(problems) != 0 {
		t.Fatalf("a module registered for zero exemptions with zero actual exemptions was rejected: %v", problems)
	}
}
