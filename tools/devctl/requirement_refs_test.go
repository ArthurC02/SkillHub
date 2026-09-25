package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func writeRefFixture(t *testing.T, extraHeadings, citer string) string {
	t.Helper()
	root := t.TempDir()
	var spec strings.Builder
	spec.WriteString("# 規格\n\n")
	for i := 1; i <= 41; i++ {
		fmt.Fprintf(&spec, "### DISC-%03d：標題\n\n允收準則：無。\n\n", i)
	}
	spec.WriteString(extraHeadings)
	writeAt(t, root, requirementSpec, spec.String())

	var work strings.Builder
	for i := 1; i <= 41; i++ {
		fmt.Fprintf(&work, "- [x] DISC-%03d 做完了\n", i)
	}
	for _, id := range requirementID.FindAllString(extraHeadings, -1) {
		fmt.Fprintf(&work, "- [x] %s 做完了\n", id)
	}
	work.WriteString(citer)
	writeAt(t, root, requirementCiters[0], work.String())
	writeAt(t, root, requirementCiters[1], "沒有引用。\n")
	writeAt(t, root, requirementCiters[2], "也沒有。\n")
	return root
}

func TestRequirementRefsAcceptsCitationsThatResolve(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t,

		"### SEC-010：安全事件回應\n\n#### SEC-010 事件嚴重度分級\n\n",
		"- [x] 做完了（允收：`02:DISC-001`、`02:SEC-010`）\n")
	if problems := requirementRefProblems(root); len(problems) != 0 {
		t.Fatalf("resolvable citations were rejected: %v", problems)
	}
}

func TestRequirementRefsReadsCitationsOutsideThePlansDirectory(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t, "", "- [x] 做完了（允收：`02:DISC-001`）\n")
	writeAt(t, root, "docs/runbooks/a-runbook.md", "帳號清除（`02:CORE-007`）必須是不可逆的終點。\n")

	problems := requirementRefProblems(root)
	if len(problems) != 1 {
		t.Fatalf("want one problem, got %d: %v", len(problems), problems)
	}
	for _, want := range []string{"docs/runbooks/a-runbook.md", "CORE-007"} {
		if !strings.Contains(problems[0], want) {
			t.Errorf("problem does not name %q: %s", want, problems[0])
		}
	}
}

func TestRequirementRefsRejectsTheThreeShapesTheTreeHasToday(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, headings, citer, want string
	}{{

		name:  "a citation to a number the spec never declares",
		citer: "見 `02:PDM-005` §5.3。\n",
		want:  "cites `02:PDM-005`, and docs/plans/02-specifications-and-acceptance-criteria.md has no heading declaring PDM-005",
	}, {

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

func TestRequirementRefsAllowsADeeperSubHeadingOfTheSameID(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t, "## SEC-010：事件回應\n\n### SEC-010 分級\n\n#### SEC-010 通知路徑\n\n",
		"見 `02:SEC-010`。\n")
	if problems := requirementRefProblems(root); len(problems) != 0 {
		t.Fatalf("one owner plus two deeper sub-headings was rejected: %v", problems)
	}
}

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

func TestRequirementRefsNamesARequirementNoWorkItemCarries(t *testing.T) {
	t.Parallel()
	carried := "- [x] 引用一次（允收：`02:DISC-001`）\n"
	root := writeRefFixture(t, "### PACK-001：打包\n\n允收準則：無。\n\n", carried)
	problems := requirementRefProblems(root)
	if len(problems) != 0 {
		t.Fatalf("a fully carried plan was rejected: %v", problems)
	}

	orphaned := writeRefFixture(t, "", carried)
	spec, err := os.ReadFile(filepath.Join(orphaned, filepath.FromSlash(requirementSpec)))
	if err != nil {
		t.Fatal(err)
	}
	writeAt(t, orphaned, requirementSpec, string(spec)+"### PACK-001：打包\n\n允收準則：無。\n\n")
	problems = requirementRefProblems(orphaned)
	if len(problems) != 1 || !strings.Contains(problems[0], "PACK-001 is MVP-required") {
		t.Fatalf("expected exactly the uncarried report for PACK-001, got %v", problems)
	}
}

func TestRequirementRefsLeavesAPostMVPRequirementAlone(t *testing.T) {
	t.Parallel()
	root := writeRefFixture(t, "", "- [x] 引用一次（允收：`02:DISC-001`）\n")
	spec, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(requirementSpec)))
	if err != nil {
		t.Fatal(err)
	}
	writeAt(t, root, requirementSpec, string(spec)+"### PACK-001：打包（後 MVP）\n\n允收準則：無。\n\n")
	if problems := requirementRefProblems(root); len(problems) != 0 {
		t.Fatalf("a 後 MVP requirement was asked for a work item: %v", problems)
	}
}
