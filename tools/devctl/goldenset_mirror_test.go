package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tree, both halves, with their pinned digests. This is the check itself;
// there is no fixture form of "these two hand-copied functions still agree".
func TestTheRealGoldensetMirrorIsStillPinned(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := goldensetMirrorProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
	// Every span must have been pinned by someone, and every pin must belong to
	// a span. A pin with no span is a pin nothing checks; a span with no pin was
	// reported above but this says so in the roster's own terms.
	if len(goldensetPinned) != len(goldensetSpans) {
		t.Fatalf("%d pinned digests for %d spans", len(goldensetPinned), len(goldensetSpans))
	}
	for _, span := range goldensetSpans {
		if goldensetPinned[span.name] == "" {
			t.Errorf("%s has no pinned digest", span.name)
		}
	}
}

// Rule 9, as a test rather than as a note: mutate each side in a copy of the
// tree and the checker must name that side. Four spans, four independent
// mutations, because a checker that watches three of four is exactly as quiet
// about the fourth as no checker at all.
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
		// Dropping the tags part: the golden set would still index task
		// examples and summaries and report a recall number as if nothing had
		// happened.
		span: "embeddingText", file: goldensetGo,
		from: "\tif tags := e.flatTags(); tags != \"\" {", to: "\tif tags := \"\"; tags != \"\" {",
	}, {
		// Reordering the buckets. Same words, different string, different
		// embedding.
		span: "flatTags", file: goldensetGo,
		from: "t.Inputs, t.Outputs, t.Tools, t.Dependencies",
		to:   "t.Outputs, t.Inputs, t.Tools, t.Dependencies",
	}, {
		// Dropping the English half of each example is the exact edit
		// golden-query-set.md §3.6 measured as the cause of both recall misses.
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

// A span whose anchor moved must be loud. Renaming embeddingText, or wrapping
// enriched_index_text in a class, would otherwise leave a green check watching
// nothing.
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

// mirrorRoot copies just the two files this check reads into a temp root, with
// one substitution applied. The real tree is never written to.
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
