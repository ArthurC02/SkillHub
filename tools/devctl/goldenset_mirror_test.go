package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheRealGoldensetMirrorIsStillPinned(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := goldensetMirrorProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}

	if len(goldensetPinned) != len(goldensetSpans) {
		t.Fatalf("%d pinned digests for %d spans", len(goldensetPinned), len(goldensetSpans))
	}
	for _, span := range goldensetSpans {
		if goldensetPinned[span.name] == "" {
			t.Errorf("%s has no pinned digest", span.name)
		}
	}
}

func TestGoldensetMirrorNoticesAChangeOnEitherSide(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		span           string
		file, from, to string
	}{{

		span: "embeddingText", file: goldensetGo,
		from: "\tif tags := e.flatTags(); tags != \"\" {", to: "\tif tags := \"\"; tags != \"\" {",
	}, {

		span: "flatTags", file: goldensetGo,
		from: "t.Inputs, t.Outputs, t.Tools, t.Dependencies",
		to:   "t.Outputs, t.Inputs, t.Tools, t.Dependencies",
	}, {

		span: "joinTaskExamples", file: goldensetGo,
		from: "lines = append(lines, ex.ZhHant, ex.En)", to: "lines = append(lines, ex.ZhHant)",
	}, {
		span: "enriched_index_text", file: goldensetPython,
		from: `return "\n".join(parts)`, to: `return " ".join(parts)`,
	}} {
		t.Run(tc.span, func(t *testing.T) {
			t.Parallel()
			mutated := mirrorRoot(t, root, tc.file, tc.from, tc.to)
			problems := goldensetMirrorProblems(mutated)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.span+" in "+tc.file+" changed") {
				t.Fatalf("mutating %s produced %v", tc.span, problems)
			}
		})
	}
}

func TestGoldensetMirrorSaysSoWhenASpanMoved(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	mutated := mirrorRoot(t, root, goldensetGo, "func embeddingText(", "func embeddedText(")
	problems := goldensetMirrorProblems(mutated)
	if len(problems) == 0 || !strings.Contains(problems[0], "lost that half of its subject") {
		t.Fatalf("a renamed function was accepted: %v", problems)
	}
}

func mirrorRoot(t *testing.T, root, target, from, to string) string {
	t.Helper()
	out := t.TempDir()
	replaced := false
	for _, rel := range []string{goldensetGo, goldensetPython} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		if rel == target {
			if !strings.Contains(body, from) {
				t.Fatalf("%s no longer contains %q, so this mutation tests nothing", rel, from)
			}
			body = strings.Replace(body, from, to, 1)
			replaced = true
		}
		path := filepath.Join(out, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !replaced {
		t.Fatalf("%s is not one of the mirrored files", target)
	}
	return out
}
