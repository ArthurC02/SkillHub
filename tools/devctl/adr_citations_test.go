package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func adrNumber(n int) string { return fmt.Sprintf("ADR-%03d", n) }

func adrCitationFixture(t *testing.T) (string, func(relative, contents string)) {
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
	one, two, nine := adrNumber(1), adrNumber(2), adrNumber(9)
	write("docs/adr/README.md", "# 索引\n\n## 決策索引\n\n### 甲主題\n\n["+one+"](./"+one+"-alpha.md)：摘要。\n\n"+
		"### 乙主題\n\n["+two+"](./"+two+"-beta.md)：摘要。\n")
	write("docs/adr/"+one+"-alpha.md", "# "+one+"：甲主題\n\n見 ["+two+"](./"+two+"-beta.md)。\n")
	write("docs/adr/"+two+"-beta.md", "# "+two+"：乙主題\n")
	write("docs/plans/01.md", "規則本身。理由見 [甲主題](../adr/README.md#甲主題)。\n")
	write("docs/plans/mvp/m1/probe.json", `{"adr": "`+nine+`"}`+"\n")
	write("docs/plans/mvp/m1/R01.SKILL.md", nine+"\n")
	write("tools/goldenset/corpus/x/SKILL.md", nine+"\n")
	write("tools/eval-regression/results.jsonl", `{"note": "`+nine+`"}`+"\n")
	write(".gitignore", "local.md\n")
	write("local.md", nine+"\n")
	return root, write
}

func TestADRCitationProblemsAcceptsACleanTree(t *testing.T) {
	t.Parallel()
	root, _ := adrCitationFixture(t)
	if problems := adrCitationProblems(root); len(problems) != 0 {
		t.Fatalf("a tree that follows every rule was rejected:\n%s", strings.Join(problems, "\n"))
	}
}

func TestADRCitationProblems(t *testing.T) {
	t.Parallel()
	one, two, seven := adrNumber(1), adrNumber(2), adrNumber(7)
	for _, c := range []struct {
		name   string
		change func(root string, write func(string, string))
		want   string
		count  int
	}{
		{
			name:   "a living document names an ADR",
			change: func(_ string, write func(string, string)) { write("docs/plans/02.md", "見 "+one+"。\n") },
			want:   "docs/plans/02.md:1 names " + one, count: 1,
		},
		{
			name: "code names an ADR in a string",
			change: func(_ string, write func(string, string)) {
				write("apps/x/a.go", "package x\n\nconst s = \"("+one+")\"\n")
			},
			want: "apps/x/a.go:3 names " + one, count: 1,
		},
		{
			name:   "a milestone record's prose is not exempt",
			change: func(_ string, write func(string, string)) { write("docs/plans/mvp/m1/report.md", "依 "+one+"。\n") },
			want:   "docs/plans/mvp/m1/report.md:1 names " + one, count: 1,
		},
		{
			name: "an ADR cites a number that has no file",
			change: func(_ string, write func(string, string)) {
				write("docs/adr/"+two+"-beta.md", "# "+two+"：乙主題\n\n見 "+seven+"。\n")
			},
			want: "docs/adr/" + two + "-beta.md:3 cites " + seven + ", and no such ADR exists", count: 1,
		},
		{
			name: "an ADR the index does not list",
			change: func(_ string, write func(string, string)) {
				write("docs/adr/"+adrNumber(3)+"-gamma.md", "# "+adrNumber(3)+"：丙主題\n")
			},
			want: "docs/adr/" + adrNumber(3) + "-gamma.md is not listed", count: 1,
		},
		{
			name: "an index heading that is not the ADR's title",
			change: func(_ string, write func(string, string)) {
				write("docs/adr/"+one+"-alpha.md", "# "+one+"：甲主題改名\n")
			},
			want: "under 「甲主題」, but the ADR is titled 「甲主題改名」", count: 1,
		},
		{
			name:   "a stray file directly in docs/adr",
			change: func(_ string, write func(string, string)) { write("docs/adr/notes.md", "筆記\n") },
			want:   "docs/adr/notes.md is in docs/adr but is not named", count: 1,
		},
		{
			name:   "a subfolder of docs/adr",
			change: func(_ string, write func(string, string)) { write("docs/adr/superseded/old.md", "舊\n") },
			want:   "docs/adr/superseded/old.md is in docs/adr but is neither an ADR nor the index", count: 1,
		},
		{
			name: "a link to an index heading that does not exist",
			change: func(_ string, write func(string, string)) {
				write("docs/plans/03.md", "[x](../adr/README.md#不存在)\n")
			},
			want: "docs/plans/03.md:1 links docs/adr/README.md#不存在", count: 1,
		},
		{
			name: "a percent-encoded anchor to an existing heading",
			change: func(_ string, write func(string, string)) {
				write("docs/plans/04.md", "[x](../adr/README.md#%E7%94%B2%E4%B8%BB%E9%A1%8C)\n")
			},
			count: 0,
		},
		{
			name: "an anchor into another README is not the index",
			change: func(_ string, write func(string, string)) {
				write("docs/plans/05.md", "[x](../design/README.md#不存在)\n")
			},
			count: 0,
		},
		{
			name: "an index without ADRs is a broken scan",
			change: func(root string, _ func(string, string)) {
				for _, name := range []string{one + "-alpha.md", two + "-beta.md"} {
					if err := os.Remove(filepath.Join(root, "docs", "adr", name)); err != nil {
						panic(err)
					}
				}
			},
			want: "docs/adr holds no ADR", count: 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root, write := adrCitationFixture(t)
			c.change(root, write)
			problems := adrCitationProblems(root)
			if len(problems) != c.count {
				t.Fatalf("want %d problems, got %d:\n%s", c.count, len(problems), strings.Join(problems, "\n"))
			}
			if c.count > 0 && !strings.Contains(problems[0], c.want) {
				t.Fatalf("problem does not contain %q:\n%s", c.want, problems[0])
			}
		})
	}
}
