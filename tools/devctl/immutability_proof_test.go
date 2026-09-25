package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeImmutabilityFixture(t *testing.T, migration, test string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string]string{
		filepath.Join("db", "migrations", "0001_fixture.sql"): migration,
		filepath.FromSlash(immutabilityTestPath):              test,
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const ledgerTrigger = "CREATE TRIGGER ledger_immutable\n" +
	"    BEFORE UPDATE OR DELETE ON ledger\n" +
	"    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();\n"

func TestImmutabilityProofAcceptsATriggerWithBothCases(t *testing.T) {
	root := writeImmutabilityFixture(t, ledgerTrigger,
		"SELECT must_fail($$UPDATE ledger SET amount = 1 WHERE id = 1$$);\n"+
			"SELECT must_fail($$DELETE FROM ledger WHERE id = 1$$);\n")
	if problems := immutabilityProofProblems(root); len(problems) != 0 {
		t.Fatalf("a fully proved trigger was rejected: %v", problems)
	}
}

func TestImmutabilityProofNamesTheGuardedOperationWithNoCase(t *testing.T) {
	for _, tc := range []struct {
		name string
		test string
		want string
	}{{
		name: "no case at all",
		test: "SELECT must_fail($$UPDATE elsewhere SET amount = 1$$);\n",
		want: "guards UPDATE on ledger",
	}, {
		name: "update proved, delete not",
		test: "SELECT must_fail($$UPDATE ledger SET amount = 1$$);\n",
		want: "guards DELETE on ledger",
	}, {
		name: "the statement runs outside a must_ helper, so nothing asserts it was refused",
		test: "UPDATE ledger SET amount = 1;\nDELETE FROM ledger WHERE id = 1;\n",
		want: "guards UPDATE on ledger",
	}, {
		name: "a table whose name only starts the same way is not the proof",
		test: "SELECT must_fail($$UPDATE ledger_entries SET amount = 1$$);\n" +
			"SELECT must_fail($$DELETE FROM ledger_entries WHERE id = 1$$);\n",
		want: "guards UPDATE on ledger",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			problems := immutabilityProofProblems(writeImmutabilityFixture(t, ledgerTrigger, tc.test))
			if len(problems) == 0 {
				t.Fatalf("an unproved guard was accepted")
			}
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("expected a problem naming %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestImmutabilityProofAsksOnlyForTheOperationsTheTriggerGuards(t *testing.T) {
	root := writeImmutabilityFixture(t,
		"CREATE TRIGGER creation_event_immutable BEFORE UPDATE ON creation_session_events\n"+
			" FOR EACH ROW EXECUTE FUNCTION forbid_creation_event_update();\n",
		"SELECT must_fail_saying($$UPDATE creation_session_events SET snapshot = '{}'$$, 'immutable');\n")
	if problems := immutabilityProofProblems(root); len(problems) != 0 {
		t.Fatalf("an UPDATE-only trigger was asked to prove a DELETE it does not guard: %v", problems)
	}
}

func TestImmutabilityProofForgetsATriggerThatWasDroppedForGood(t *testing.T) {
	root := writeImmutabilityFixture(t,
		ledgerTrigger+"\nDROP TRIGGER ledger_immutable ON ledger;\n",
		"-- nothing left to prove\n")
	if problems := immutabilityProofProblems(root); len(problems) != 1 ||
		!strings.Contains(problems[0], "has lost its subject") {
		t.Fatalf("expected only the lost-subject report, got %v", problems)
	}
}

func TestImmutabilityProofReadsTheLiveMigrationsAndTest(t *testing.T) {
	t.Parallel()
	const root = "../.."
	live, problems := liveImmutableTriggers(root)
	if len(problems) != 0 {
		t.Fatalf("reading the live migrations reported problems: %v", problems)
	}
	for _, name := range []string{"cost_events_immutable", "credit_entries_immutable",
		"creation_event_immutable", "evaluation_model_usage_immutable",
		"evaluation_suggestion_applications_immutable", "run_attempts_immutable"} {
		if _, ok := live[name]; !ok {
			t.Fatalf("%s is declared in db/migrations but the checker did not see it", name)
		}
	}
	if problems := immutabilityProofProblems(root); len(problems) != 0 {
		t.Fatalf("the live tree has an unproved immutability guard: %v", problems)
	}
}
