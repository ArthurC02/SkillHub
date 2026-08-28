package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestMain builds a fake package under root so the check can be pointed at
// a tree whose contents this test controls.
func writeTestMain(t *testing.T, root, pkg, body string) string {
	t.Helper()
	dir := filepath.Join(root, "apps", "platform", "internal", pkg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "x_test.go")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const guarded = `package p
func TestMain(m *testing.M) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" { os.Exit(1) }
		os.Exit(m.Run())
	}
}`

const unguarded = `package p
func TestMain(m *testing.M) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		os.Exit(m.Run())
	}
}`

func TestRequireDBGuardCatchesAPackageThatIgnoresTheSwitch(t *testing.T) {
	root := t.TempDir()
	writeTestMain(t, root, "good", guarded)
	writeTestMain(t, root, "bad", unguarded)

	missing, err := unguardedDBTestMains(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "bad") {
		t.Fatalf("got %v, want only the unguarded package", missing)
	}
	err = requireDBGuardCheck(root)
	if err == nil {
		t.Fatal("check passed with an unguarded database TestMain; it has no teeth")
	}
	if !strings.Contains(err.Error(), "PORT-004") {
		t.Errorf("the failure should say which requirement it enforces: %v", err)
	}
}

func TestRequireDBGuardIgnoresTestMainsWithNoDatabase(t *testing.T) {
	root := t.TempDir()
	// A TestMain that never mentions the database URL is not this check's
	// business; flagging it would train people to add the switch as noise.
	writeTestMain(t, root, "nodb", `package p
func TestMain(m *testing.M) { os.Exit(m.Run()) }`)
	if err := requireDBGuardCheck(root); err != nil {
		t.Errorf("flagged a TestMain that has nothing to do with the database: %v", err)
	}
}

// TestTheRealRepositoryPassesIsTheOnePeopleWillSee keeps the check pointed at
// the tree it exists for, not only at fixtures.
func TestTheRealRepositoryPassesIsTheOnePeopleWillSee(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("not inside the repository: %v", err)
	}
	if err := requireDBGuardCheck(root); err != nil {
		t.Error(err)
	}
}
