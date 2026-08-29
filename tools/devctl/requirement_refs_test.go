package main

import (
	"fmt"
	"strings"
	"testing"
)

// Pointed at the tree. Expected to be RED until the three dangling citations are
// rewritten (`02:PDM-005`, `02:SBX-008`, `02:736-759`), so the failure names
// them rather than just failing.
func TestTheRealSpecCitationsAllResolve(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := requirementRefProblems(root); len(problems) > 0 {
		t.Fatalf("%d unresolved citation(s) or ambiguous heading(s):\n%s",
			len(problems), strings.Join(problems, "\n"))
	}
}

// The SEC-010 shape must stay accepted whatever else changes, so it gets its own
// assertion against the real spec rather than only a fixture.
func TestTheRealSpecKeepsExactlyTheDocumentedRepeatedID(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	headings, problems := specHeadingIDs(root + "/" + requirementSpec)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if len(headings) < 40 {
		t.Fatalf("only %d heading ids found; the scan is looking at the wrong thing", len(headings))
	}
	for id, at := range headings {
		if len(at) == 1 {
			continue
		}
		if id != "SEC-010" {
			t.Errorf("%s is declared %d times (%v); only SEC-010's `###` + nested `####` shape is documented",
				id, len(at), at)
			continue
		}
		if len(at) != 2 || at[0].depth != 3 || at[1].depth != 4 {
			t.Errorf("SEC-010's shape changed: %v; the rule allows one shallowest heading plus strictly "+
				"deeper sub-headings", at)
		}
	}
}

// spec builds a fixture spec with enough headings to clear the scan floor.
func writeRefFixture(t *testing.T, extraHeadings, citer string) string {
	t.Helper()
	root := t.TempDir()
	var spec strings.Builder
	spec.WriteString("# 規格\n\n")
	for i := 1; i <= 41; i++ {
		spec.WriteString(fmt.Sprintf("### DISC-%03d：標題\n\n允收準則：無。\n\n", i))
	}
	spec.WriteString(extraHeadings)
	writeAt(t, root, requirementSpec, spec.String())
	writeAt(t, root, requirementCiters[0], citer)
	writeAt(t, root, requirementCiters[1], "沒有引用。\n")
	writeAt(t, root, requirementCiters[2], "也沒有。\n")
	return root
}

func TestRequirementRefsAcceptsCitationsThatResolve(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t,
		// The documented SEC-010 shape: one `###` owner, one deeper `####` child.
		"### SEC-010：安全事件回應\n\n#### SEC-010 事件嚴重度分級\n\n",
		"- [x] 做完了（允收：`02:DISC-001`、`02:SEC-010`）\n")
	if problems := requirementRefProblems(root); len(problems) != 0 {
		t.Fatalf("resolvable citations were rejected: %v", problems)
	}
}

func TestRequirementRefsRejectsTheThreeShapesTheTreeHasToday(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, headings, citer, want string
	}{{
		// 02:PDM-005 — a number defined in an m0 proposal, cited as a spec id.
		name:  "a citation to a number the spec never declares",
		citer: "見 `02:PDM-005` §5.3。\n",
		want:  "cites `02:PDM-005`, and docs/plans/02-specifications-and-acceptance-criteria.md has no heading declaring PDM-005",
	}, {
		// 02:736-759 — a line range in requirement-id notation.
		name:  "a line range written as a requirement id",
		citer: "`02:736-759` 從頭到尾沒提過 Q16。\n",
		want:  "cites `02:736-759`",
	}, {
		name:     "the same id owning two sections at the same depth",
		headings: "### PACK-001：一\n\n### PACK-001：二\n\n",
		citer:    "見 `02:DISC-001`。\n",
		want:     "declares PACK-001 in 2 headings at the same depth",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := requirementRefProblems(writeRefFixture(t, tc.headings, tc.citer))
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// A nested repeat is the allowed shape; a same-depth repeat is not. Both
// directions, because an over-strict rule would force the SEC-010 sub-heading to
// be renamed and an over-loose one enforces nothing.
func TestRequirementRefsAllowsADeeperSubHeadingOfTheSameID(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t, "## SEC-010：事件回應\n\n### SEC-010 分級\n\n#### SEC-010 通知路徑\n\n",
		"見 `02:SEC-010`。\n")
	if problems := requirementRefProblems(root); len(problems) != 0 {
		t.Fatalf("one owner plus two deeper sub-headings was rejected: %v", problems)
	}
}

// Both halves lose their subject in ways that are green under a naive
// implementation: no headings means every citation resolves to nothing and
// nothing to compare, no citations means the comparison never runs.
func TestRequirementRefsSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("the heading scan finds almost nothing", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeAt(t, root, requirementSpec, "# 規格\n\n### DISC-001：標題\n")
		for _, c := range requirementCiters {
			writeAt(t, root, c, "見 `02:DISC-001`。\n")
		}
		problems := requirementRefProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "heading scan is broken") {
			t.Fatalf("a nearly empty spec was accepted: %v", problems)
		}
	})
	t.Run("the citation scan finds nothing", func(t *testing.T) {
		t.Parallel()
		root := writeRefFixture(t, "", "沒有任何引用。\n")
		problems := requirementRefProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "citation scan is broken") {
			t.Fatalf("zero citations was accepted: %v", problems)
		}
	})
	t.Run("the spec is gone", func(t *testing.T) {
		t.Parallel()
		if problems := requirementRefProblems(t.TempDir()); len(problems) == 0 {
			t.Fatal("a missing spec was accepted")
		}
	})
}
