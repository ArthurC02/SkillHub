package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocLinkProblems(t *testing.T) {
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

	write("docs/adr/ADR-011-workspace-tenancy-policy-and-usage.md", "# ADR-011\n")
	write("docs/plans/01.md", strings.Join([]string{
		// Resolves.
		"見 [ADR-011](../adr/ADR-011-workspace-tenancy-policy-and-usage.md)。",
		// The real defect: a name that describes the subject correctly.
		"見 [ADR-011](../adr/ADR-011-workspace-scope-and-tenancy.md)。",
		// Anchors, external URLs and mail are out of scope, and an anchor on a
		// path that exists must not be read as part of the filename.
		"[跳到](#§10) [外部](https://example.com/x.md) [信](mailto:a@b.c)",
		"[有錨點](../adr/ADR-011-workspace-tenancy-policy-and-usage.md#決策)",
		// Percent-encoded space: the file exists, the raw link does not look
		// like it does.
		"[空格](./有 空格.md)",
		// AGENTS.md's ADR-reference rule, verbatim. The parentheses are a blank
		// to fill in, and Windows resolves `...` while Linux does not — this
		// line is the one that turned a green local run into a red CI run.
		"回填 → [ADR-xxx](...) 引用",
	}, "\n")+"\n")
	write("docs/plans/有 空格.md", "x\n")
	// Generated trees and the frozen golden-set corpus are not walked.
	write("packages/api-client-ts/README.md", "[gen](docs/DefaultApi.md)\n")
	write("tools/goldenset/corpus/data/x.md", "[ref](references/nope.md)\n")

	problems := docLinkProblems(root)
	if len(problems) != 1 {
		t.Fatalf("want exactly the one dead link, got %d:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	want := "docs/plans/01.md:2"
	if !strings.Contains(problems[0], want) {
		t.Fatalf("problem does not point at %s: %s", want, problems[0])
	}
	if !strings.Contains(problems[0], "ADR-011-workspace-scope-and-tenancy.md") {
		t.Fatalf("problem does not name the target: %s", problems[0])
	}
}

// The `(...)` above cannot be judged by asking the filesystem: Windows resolves
// `...` and Linux does not, so the fixture test passes on this machine with or
// without the guard, and CI is where you find out. Assert on the decision
// itself, which has no operating system.
func TestDocLinkTargetIgnoresProsePlaceholders(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"...", ".", ".."} {
		if _, ok := docLinkTarget(raw); ok {
			t.Fatalf("%q was accepted as a path; it is AGENTS.md's fill-in-the-blank for an ADR reference", raw)
		}
	}
	if _, ok := docLinkTarget("./x.md"); !ok {
		t.Fatal("a real relative target was rejected")
	}
}
