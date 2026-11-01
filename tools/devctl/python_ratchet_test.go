package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pythonRatchetFixture(t *testing.T, baseline string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range files {
		writeTestFile(t, root, filepath.FromSlash(file), "print('x')\n")
	}
	writeTestFile(t, root, filepath.FromSlash(pythonRatchetFile), baseline)
	return root
}

func TestPythonRatchetAcceptsExactlyTheListedFiles(t *testing.T) {
	t.Parallel()
	root := pythonRatchetFixture(t, "tools/a/one.py\n\ntools/b/two.py\n", "tools/a/one.py", "tools/b/two.py")
	if problems := pythonRatchetProblems(root); len(problems) != 0 {
		t.Fatalf("pythonRatchetProblems() = %v, want none", problems)
	}
}

func TestPythonRatchetNamesANewPythonFileAndWhereToolingBelongs(t *testing.T) {
	t.Parallel()
	root := pythonRatchetFixture(t, "tools/a/one.py\n", "tools/a/one.py", "tools/c/new.py")
	problems := pythonRatchetProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "tools/c/new.py is a new Python file") || !strings.Contains(problems[0], "tools/devctl") {
		t.Fatalf("pythonRatchetProblems() = %v, want the new file named with where tooling belongs", problems)
	}
}

func TestPythonRatchetAsksToDropALineWhoseFileIsGone(t *testing.T) {
	t.Parallel()
	root := pythonRatchetFixture(t, "tools/a/one.py\ntools/gone.py\n", "tools/a/one.py")
	problems := pythonRatchetProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "tools/gone.py no longer exists") {
		t.Fatalf("pythonRatchetProblems() = %v, want the stale line named", problems)
	}
}

func TestPythonRatchetIgnoresInstalledAndBuiltDirectoriesAndOtherTrees(t *testing.T) {
	t.Parallel()
	root := pythonRatchetFixture(t, "tools/a/one.py\n",
		"tools/a/one.py",
		"tools/x/node_modules/pkg/setup.py",
		"tools/x/.venv/lib/site.py",
		"tools/x/__pycache__/m.py",
		"tools/x/target/gen.py",
		"tools/x/dist/build.py",
		"apps/llm/main.py",
	)
	if problems := pythonRatchetProblems(root); len(problems) != 0 {
		t.Fatalf("pythonRatchetProblems() = %v, want none", problems)
	}
}

func TestPythonRatchetReportsAMissingBaseline(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	problems := pythonRatchetProblems(root)
	if len(problems) != 1 || !strings.HasPrefix(problems[0], pythonRatchetFile+":") {
		t.Fatalf("pythonRatchetProblems() = %v, want the baseline file named", problems)
	}
}
