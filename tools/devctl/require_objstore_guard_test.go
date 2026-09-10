package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
