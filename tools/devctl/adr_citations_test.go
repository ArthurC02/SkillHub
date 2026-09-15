package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const adrFixtureCitations = 3

func adrCitationFixture(t *testing.T, rows int) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	status := map[int]string{
		1:  "Superseded（由 [ADR-002](./ADR-002-x.md)）",
		3:  "Accepted（決策 1 經 ADR-004 修訂）",
		50: "Superseded",
	}
	var index strings.Builder
	index.WriteString("| ADR | 主題 | 狀態 |\n| --- | --- | --- |\n")
	for n := 0; n < rows; n++ {
		s := status[n]
		if s == "" {
			s = "Accepted"
		}
		fmt.Fprintf(&index, "| [ADR-%03d](./ADR-%03d-x.md) | 主題 | %s |\n", n, n, s)
	}
	write("docs/adr/README.md", index.String())

	write("docs/adr/ADR-004-x.md", "# ADR-004\n\n- 狀態：Accepted\n- 修訂：[ADR-003](./ADR-003-x.md) 決策 1\n\n## 背景\n\n見 ADR-001。\n")
	write("docs/adr/ADR-006-x.md", "# ADR-006\n\n- 修訂：[ADR-005](./ADR-005-x.md) 決策 2（與 ADR-003 相容）\n")
	write("docs/adr/ADR-007-x.md", "# ADR-007\n\n- 修訂：由 [ADR-009](./ADR-009-x.md) 修訂\n")
	write("docs/adr/ADR-008-x.md", "# ADR-008\n\n## 決策\n\n- 修訂：[ADR-005](./ADR-005-x.md)\n")

	write("docs/plans/01.md", "見 ADR-001。\nADR-001 已由 ADR-002 取代。\n")
	write("docs/plans/mvp/m1/report.md", "見 ADR-001。\n")

	write("apps/x/a.go", "package x\n\nconst see = \"ADR-010\"\nconst path = \"docs/adr/ADR-011-workspace.md\"\n")
	write("contracts/c.yaml", "summary: x (ADR-015, ADR-016)\n")
	write("apps/x/gen/b.go", "package gen\n\nconst see = \"ADR-001 ADR-012\"\n")
	write("tools/r.jsonl", "{\"note\": \"ADR-013\"}\n")
	write(".gitignore", "ignored.go\n")
	write("ignored.go", "package x\n\nconst see = \"ADR-020 ADR-001\"\n")
	return root
}

func adrProblemContaining(problems []string, want string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return true
		}
	}
	return false
}

func TestADRCitationProblemsReportsEachBrokenLinkOnce(t *testing.T) {
	t.Parallel()
	root := adrCitationFixture(t, adrIndexRowFloor)
	want := []string{
		"marks ADR-050 Superseded without naming the ADR that superseded it",
		"ADR-006-x.md says 「修訂」 ADR-005, and the docs/adr/README.md row of ADR-005 does not name ADR-006",
		"docs/plans/01.md:1 cites ADR-001, which docs/adr/README.md says is superseded, without naming ADR-002",
	}
	problems := adrCitationProblemsWithin(root, adrFixtureCitations)
	if len(problems) != len(want) {
		t.Fatalf("want %d problems, got %d:\n%s", len(want), len(problems), strings.Join(problems, "\n"))
	}
	for _, w := range want {
		if !adrProblemContaining(problems, w) {
			t.Fatalf("no problem contains %q:\n%s", w, strings.Join(problems, "\n"))
		}
	}
}

func TestADRCitationCeilingIsExact(t *testing.T) {
	t.Parallel()
	root := adrCitationFixture(t, adrIndexRowFloor)
	for _, c := range []struct {
		name    string
		ceiling int
		want    string
	}{
		{"count equal to the ceiling passes", adrFixtureCitations, ""},
		{"count one above the ceiling fails", adrFixtureCitations - 1, "3 times, above the ceiling of 2"},
		{"count one below the ceiling asks to lower it", adrFixtureCitations + 1,
			"below the ceiling of 4; lower adrCitationCeiling in tools/devctl/adr_citations.go to 3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ratchet []string
			for _, problem := range adrCitationProblemsWithin(root, c.ceiling) {
				if strings.Contains(problem, "ceiling of") {
					ratchet = append(ratchet, problem)
				}
			}
			if c.want == "" {
				if len(ratchet) != 0 {
					t.Fatalf("want no ratchet problem, got %q", ratchet)
				}
				return
			}
			if len(ratchet) != 1 || !strings.Contains(ratchet[0], c.want) {
				t.Fatalf("want one ratchet problem containing %q, got %q", c.want, ratchet)
			}
		})
	}
}

func TestADRCitationIndexBelowTheFloorIsABrokenScan(t *testing.T) {
	t.Parallel()
	problems := adrCitationProblemsWithin(adrCitationFixture(t, adrIndexRowFloor-1), adrFixtureCitations)
	if !adrProblemContaining(problems, "has 79 index rows") {
		t.Fatalf("an index one row under the floor was accepted:\n%s", strings.Join(problems, "\n"))
	}
}
