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
