package main

import (
	"strings"
	"testing"
)

func TestCapabilityLedgerHasNoUnexplainedVariables(t *testing.T) {
	t.Parallel()
	for _, bucket := range capabilityLedger {
		if len(bucket.vars) == 0 {
			continue
		}
		if len(bucket.reason) > 0 && []rune(bucket.reason)[0] == '⛔' {
			t.Errorf("ledger bucket %q still has unexplained variables: %v", bucket.reason, bucket.vars)
		}
	}
}

// writeFakeDeclaredCapabilities stands up a minimal Go module at
// root/apps/platform whose `go run ./cmd/api --capabilities` prints capsJSON,
// so declaredCapabilityVars can be exercised without the real platform binary.
func writeFakeDeclaredCapabilities(t *testing.T, root, capsJSON string) {
	t.Helper()
	writeTestFile(t, root, "apps/platform/go.mod", "module fakeplatform\n\ngo 1.21\n")
	writeTestFile(t, root, "apps/platform/cmd/api/main.go",
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Print(`"+capsJSON+"`)\n}\n")
}

func TestCapabilityTableProblemsCatchesTheThreeWaysTheTableDrifts(t *testing.T) {
	t.Run("an undeclared, unexcused var in .env.example", func(t *testing.T) {
		root := t.TempDir()
		writeFakeDeclaredCapabilities(t, root, `[{"id":"x","needs":["KNOWN_VAR"]}]`)
		writeTestFile(t, root, ".env.example", "KNOWN_VAR=1\nSURPRISE_VAR=1\n")
		problems := strings.Join(capabilityTableProblems(root), "\n")
		if !strings.Contains(problems, "SURPRISE_VAR does not say what it blocks") {
			t.Fatalf("an undeclared, unexcused var was accepted: %v", problems)
		}
	})

	t.Run("a declared var missing from .env.example", func(t *testing.T) {
		root := t.TempDir()
		writeFakeDeclaredCapabilities(t, root, `[{"id":"x","needs":["MISSING_VAR"]}]`)
		writeTestFile(t, root, ".env.example", "OTHER_VAR=1\n")
		problems := strings.Join(capabilityTableProblems(root), "\n")
		if !strings.Contains(problems, "capability table names MISSING_VAR, which .env.example does not document") {
			t.Fatalf("a declared var missing from .env.example was accepted: %v", problems)
		}
	})

	t.Run("a ledger-excused var no longer in .env.example", func(t *testing.T) {
		root := t.TempDir()
		writeFakeDeclaredCapabilities(t, root, `[{"id":"x","needs":[]}]`)
		writeTestFile(t, root, ".env.example", "OTHER_VAR=1\n")
		problems := strings.Join(capabilityTableProblems(root), "\n")
		if !strings.Contains(problems, "capabilityLedger excuses APP_URL") {
			t.Fatalf("a stale ledger excuse was accepted: %v", problems)
		}
	})
}
