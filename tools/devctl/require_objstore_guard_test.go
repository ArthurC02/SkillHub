package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeObjstoreTest plants one _test.go in the directory the check names, so
// each case below is a whole tree the check can be pointed at.
func writeObjstoreTest(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(objstoreTestDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const objstoreGuarded = `package objstore

const objstoreEndpointEnv = "SKILLHUB_TEST_OBJSTORE_ENDPOINT"

func TestMain(m *testing.M) {
	if os.Getenv(objstoreEndpointEnv) == "" {
		if os.Getenv("SKILLHUB_REQUIRE_OBJSTORE") == "1" { os.Exit(1) }
		os.Exit(m.Run())
	}
	os.Exit(m.Run())
}`

func TestRequireObjstoreGuardAcceptsAGuardedPackage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeObjstoreTest(t, root, "presign_integration_test.go", objstoreGuarded)
	if problems := requireObjstoreGuardProblems(root); len(problems) != 0 {
		t.Fatalf("a guarded package was flagged: %#v", problems)
	}
}

// The first of the two deaths 02:PORT-009 names: the switch goes away, an
// object store that never came up skips everything, and go test prints ok.
func TestRequireObjstoreGuardCatchesADroppedSwitch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeObjstoreTest(t, root, "presign_integration_test.go", `package objstore

const objstoreEndpointEnv = "SKILLHUB_TEST_OBJSTORE_ENDPOINT"

func TestMain(m *testing.M) { os.Exit(m.Run()) }`)
	problems := requireObjstoreGuardProblems(root)
	if len(problems) != 1 {
		t.Fatalf("expected one problem for a package that ignores the switch, got %#v", problems)
	}
	if !strings.Contains(problems[0], "SKILLHUB_REQUIRE_OBJSTORE") ||
		!strings.Contains(problems[0], "PORT-009") {
		t.Errorf("the failure should name the switch and the requirement: %q", problems[0])
	}
}

// A comment is not a guard. The sibling check shipped with exactly this hole —
// it searched raw file text, so its own explanatory comment satisfied it — and
// the fix (reprint from the AST, dropping comments) is shared code, which means
// it can be un-shared. This is the assertion that would notice.
func TestRequireObjstoreGuardIsNotSatisfiedByAComment(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeObjstoreTest(t, root, "presign_integration_test.go", `package objstore

const objstoreEndpointEnv = "SKILLHUB_TEST_OBJSTORE_ENDPOINT"

// CI sets os.Getenv("SKILLHUB_REQUIRE_OBJSTORE") so this cannot be skipped.
func TestMain(m *testing.M) { os.Exit(m.Run()) }`)
	if problems := requireObjstoreGuardProblems(root); len(problems) != 1 {
		t.Fatalf("a comment mentioning the switch passed the check: %#v", problems)
	}
}

// The second death, and the one no switch-checking alone would see: the whole
// file goes away. A tree with no gated package has nothing to complain about,
// so the check has to name the directory it expects to find one in.
func TestRequireObjstoreGuardCatchesTheTestBeingDeleted(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeObjstoreTest(t, root, "objstore_test.go", `package objstore

func TestSomethingElse(t *testing.T) {}`)
	problems := requireObjstoreGuardProblems(root)
	if len(problems) != 1 {
		t.Fatalf("expected one problem for a package with no real-S3 test, got %#v", problems)
	}
	if !strings.Contains(problems[0], "SBX-008") {
		t.Errorf("the failure should say what stopped being proven: %q", problems[0])
	}
}

// Pointed at the tree it exists for, not only at fixtures.
func TestRequireObjstoreGuardPassesOnTheRealRepository(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("not inside the repository: %v", err)
	}
	if problems := requireObjstoreGuardProblems(root); len(problems) != 0 {
		t.Error(strings.Join(problems, "\n"))
	}
}
