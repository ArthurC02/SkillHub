package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every checker on the roster must actually reach the report.
//
// Until this existed, automationCheck had no test at all: each checker was
// tested by calling it directly, and the seven `append` lines that ran them were
// covered by nothing. Deleting the queryOwnerProblems line kept `go test ./...`
// green while ADR-033's ratchet quietly stopped running, and the same was true
// of every other line.
//
// The fixture is a repo broken in every direction on purpose, so each checker has
// something to say. Two assertions, and the second is the one with teeth: the
// checker must produce a problem here (otherwise the wiring assertion would be
// vacuously true), and every problem it produces must appear in the output.
// The roster itself, named. TestAutomationCheckRunsEveryChecker below walks
// documentCheckers(), so deleting an entry would delete its subtest with it and
// stay green — the same shape as the hole it was written to close. This is the
// second author the list needs: removing a checker is now two edits, and one of
// them is in a test file where "why is this line going away" has to be answered.
func TestDocumentCheckerRosterIsComplete(t *testing.T) {
	t.Parallel()
	want := []string{
		"drift-marker", "depguard-deny", "one-number", "query-owner",
		"context-map", "doc-identifier", "milestone-tally", "backlog-tally",
		"baseline-tally", "retention-floor", "sdk-version", "single-data-layer",
		// The two that were wired by a bare `append` below the loop until
		// 2026-08-29, so this roster walked past them and unwiring BOTH left the
		// package green. They were the highest-value pair on the list.
		"require-db-guard",
		// 02:PORT-009's twin of require-db-guard, and the only reason SBX-008's
		// short-lived authorization is proven anywhere: it watches both the
		// switch and the existence of the test file.
		"require-objstore-guard",
		"isolation-level",
		"route-table", "requirement-refs", "purge-schedule", "timeout-budget",
		"image-version", "embedding-dims", "goldenset-mirror",
		// 05 R-36 第二段: every deployment variable says what it blocks.
		"capability-table",
	}
	got := make([]string, 0, len(want))
	for _, checker := range documentCheckers() {
		got = append(got, checker.name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the checker roster changed: got [%s], want [%s]. "+
			"Adding a checker means adding it here; removing one means saying so here too",
			strings.Join(got, ", "), strings.Join(want, ", "))
	}
}

func TestAutomationCheckRunsEveryChecker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// automationCheck returns early when the Taskfile cannot be read, before any
	// checker runs; everything else is deliberately missing or wrong.
	write("Taskfile.yml", "version: \"3\"\ntasks:\n")
	// doc-identifier only speaks when a live document names something no file
	// declares, so give it one.
	write("AGENTS.md", "AGENTS 導覽：`NoSuchSymbolAnywhere` 早就被刪掉了。\n")
	// drift-marker returns on the first unreadable file and reads its two sources
	// out of a map, so with both missing its message depends on map order. Both
	// present and disagreeing gives it one thing to say.
	write("apps/platform/.golangci.yml", "# drift: DDD-005 (run -> eval)\n")
	write("docs/adr/"+contextMapADR, "# ADR-032\n\n沒有 §1 表格，也沒有附錄 A。\n")
	// single-data-layer needs something to compare against and something that
	// looks like a replica of it. Three methods, name and signature both
	// copied verbatim from the shape db/gen actually generates, is the
	// smallest tree that gives the checker something to say.
	write(genDirRelative+"/fake.sql.go", `package gen

import "context"

type Queries struct{}

func (q *Queries) CreateUser(ctx context.Context, arg CreateUserParams) (User, error) {
	return User{}, nil
}
func (q *Queries) GetUser(ctx context.Context, id int64) (User, error) {
	return User{}, nil
}
func (q *Queries) DeleteUser(ctx context.Context, id int64) error {
	return nil
}

type CreateUserParams struct{ Email string }
type User struct{ ID int64 }
`)
	write("apps/platform/internal/fake/mem.go", `package fake

import (
	"context"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type memQueries struct{}

func (m *memQueries) CreateUser(ctx context.Context, arg gen.CreateUserParams) (gen.User, error) {
	return gen.User{}, nil
}
func (m *memQueries) GetUser(ctx context.Context, id int64) (gen.User, error) {
	return gen.User{}, nil
}
func (m *memQueries) DeleteUser(ctx context.Context, id int64) error {
	return nil
}
`)

	// timeout-budget is the one checker with nothing to say about an absent
	// tree: no markers means no pairs and no complaint. A one-sided marker is
	// what it exists to catch, so that is what the fixture gives it.
	write("apps/platform/budgets.go",
		"package x\n\nconst t = 135 * time.Second // budget-over: nothing.PAIRS_WITH_THIS\n")

	// require-db-guard needs a package that gates itself on the database URL and
	// hands straight to m.Run(). Without one it has nothing to say, and the
	// wiring assertion below would be vacuously true for the checker whose
	// absence 02:PORT-004 was written about.
	write("apps/platform/internal/fake/main_test.go", `package fake

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKILLHUB_TEST_DATABASE_URL") == "" {
		return
	}
	os.Exit(m.Run())
}
`)

	var out bytes.Buffer
	if err := automationCheck(root, &out); err == nil {
		t.Fatal("a repo missing every input was accepted")
	}
	report := out.String()

	for _, checker := range documentCheckers() {
		t.Run(checker.name, func(t *testing.T) {
			problems := checker.check(root)
			if len(problems) == 0 {
				t.Fatalf("%s reported nothing on a broken tree, so the wiring assertion below proves nothing",
					checker.name)
			}
			for _, problem := range problems {
				if !strings.Contains(report, problem) {
					t.Fatalf("%s is not wired into automationCheck: %q is missing from the report",
						checker.name, problem)
				}
			}
		})
	}
}

func TestDriftMarkerProblems(t *testing.T) {
	t.Parallel()
	// The ADR fixture carries the `DDD-00x` placeholder from ADR-032 §3 on
	// purpose: it is prose, not a marker, and must not be counted.
	write := func(root, lint, adr string) {
		lintPath := filepath.Join(root, "apps", "platform", ".golangci.yml")
		adrPath := filepath.Join(root, "docs", "adr", "ADR-032-ddd-bounded-context-governance-for-platform.md")
		for path, contents := range map[string]string{lintPath: lint, adrPath: adr} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	matched := t.TempDir()
	write(matched,
		"# drift: DDD-005 (run -> eval)\n# drift: DDD-006 (run -> ingest)\n# drift: DDD-006 (eval -> ingest)\n",
		"標註（`# drift: DDD-00x`）\n| **drift: DDD-005** |\n| **drift: DDD-006** |\n| **drift: DDD-006** |\n")
	if problems := driftMarkerProblems(matched); len(problems) != 0 {
		t.Fatalf("matching markers reported problems: %#v", problems)
	}

	skewed := t.TempDir()
	write(skewed,
		"# drift: DDD-005 (run -> eval)\n# drift: DDD-006 (run -> ingest)\n# drift: DDD-006 (eval -> ingest)\n",
		"| **drift: DDD-005** |\n| **drift: DDD-006** |\n")
	problems := driftMarkerProblems(skewed)
	if len(problems) != 1 {
		t.Fatalf("expected one problem for a marker only present in the lint config, got %#v", problems)
	}
	if !strings.Contains(problems[0], "lint=2 adr=1") {
		t.Fatalf("problem does not report the count difference: %q", problems[0])
	}
}

func TestTaskDescriptionsFindsMissingDescriptions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "Taskfile.yml")
	contents := `version: "3"
tasks:
  documented:
    desc: Safe task
    cmd: echo ok
  missing:
    cmd: echo hidden
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskDescriptions(path)
	if err != nil {
		t.Fatal(err)
	}
	if tasks["documented"] != "Safe task" {
		t.Fatalf("documented description = %q", tasks["documented"])
	}
	if description, ok := tasks["missing"]; !ok || description != "" {
		t.Fatalf("missing task was not detected: %#v", tasks)
	}
}
