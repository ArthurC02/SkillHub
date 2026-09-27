package main

import (
	"fmt"
	"strings"
	"testing"
)

func complexityFixtureLint(enableGocognit bool, minComplexity string, exemptionCount int) string {
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

func withComplexityFixture(t *testing.T, contents string) string {
	t.Helper()
	root := t.TempDir()
	const lintPath = "fixture/.golangci.yml"
	writeAt(t, root, lintPath, contents)

	original := complexityGateLintPaths
	complexityGateLintPaths = []string{lintPath}
	t.Cleanup(func() { complexityGateLintPaths = original })
	return root
}

func TestComplexityExemptionsAcceptsAModuleThatExemptsNoFunction(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(true, "30", 0))
	if problems := complexityExemptionProblems(root); len(problems) != 0 {
		t.Fatalf("a module with no per-function exemption at the ceiling was rejected: %v", problems)
	}
}

func TestComplexityExemptionsRejectsASingleExemptedFunction(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(true, "30", 1))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "exempts 1 functions by name; none are allowed") {
		t.Fatalf("a module that exempts one function by name was accepted: %v", problems)
	}
}

func TestComplexityExemptionsCountsEveryExemptedFunction(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(true, "30", 3))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "exempts 3 functions by name") {
		t.Fatalf("three exempted functions were not counted as three: %v", problems)
	}
}

func TestComplexityExemptionsRejectsGocognitNotEnabled(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(false, "30", 0))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "does not enable gocognit") {
		t.Fatalf("a module that never enables gocognit was accepted: %v", problems)
	}
}

func TestComplexityExemptionsRejectsAMissingMinComplexity(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(true, "", 0))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "sets no gocognit min-complexity") {
		t.Fatalf("a module with no min-complexity setting was accepted: %v", problems)
	}
}

func TestComplexityExemptionsRejectsOneOverTheCeilingBoundary(t *testing.T) {
	root := withComplexityFixture(t, complexityFixtureLint(true, "31", 0))
	problems := complexityExemptionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "sets min-complexity 31, above the registered ceiling 30") {
		t.Fatalf("a raised gocognit ceiling was accepted: %v", problems)
	}
}

func TestComplexityExemptionsReportsAMissingLintFile(t *testing.T) {
	root := t.TempDir()
	original := complexityGateLintPaths
	complexityGateLintPaths = []string{"does/not/exist.yml"}
	t.Cleanup(func() { complexityGateLintPaths = original })

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
		t.Fatalf("a .golangci.yml in this repo excuses a function or loosens the gate: %v", problems)
	}
}
