package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baselineValidOwnerBody = "" +
	"| C-01 | desc | 阻擋 | note |\n" +
	"| C-02 | desc | 阻擋 | note |\n" +
	"| P-01 | desc | 告警 | note |\n" +
	"\n" +
	"合計：3 項檢查（阻擋 2 項、告警 1 項）\n" +
	"\n" +
	"| 分區 | 總數 | 阻擋 | 告警 |\n" +
	"| C 名稱 | 2 | 2 | 0 |\n" +
	"| P 名稱 | 1 | 0 | 1 |\n" +
	"| **合計** | **3** | **2** | **1** |\n"

func writeBaseline(t *testing.T, owner string, quoterBodies ...string) string {
	t.Helper()
	root := t.TempDir()
	ownerDir := filepath.Join(root, "docs", "plans", "mvp", "m0")
	if err := os.MkdirAll(ownerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ownerDir, "threat-model-and-sandbox-baseline.md"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, quoter := range baselineQuoters {
		body := ""
		if i < len(quoterBodies) {
			body = quoterBodies[i]
		}
		path := filepath.Join(root, filepath.FromSlash(quoter))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestBaselineTallyAcceptsAMatchingBaseline(t *testing.T) {
	root := writeBaseline(t, baselineValidOwnerBody)
	if problems := baselineTallyProblems(root); len(problems) != 0 {
		t.Fatalf("a correct baseline was rejected: %v", problems)
	}
}

func TestBaselineTallyRejectsDrift(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner string
		want  string
	}{{
		name:  "合計 total disagrees with the rows",
		owner: strings.Replace(baselineValidOwnerBody, "合計：3 項檢查（阻擋 2 項、告警 1 項）", "合計：4 項檢查（阻擋 2 項、告警 1 項）", 1),
		want:  "but its rows are 3",
	}, {
		name:  "the zone table's own 合計 row disagrees with the rows",
		owner: strings.Replace(baselineValidOwnerBody, "| **合計** | **3** | **2** | **1** |", "| **合計** | **4** | **2** | **1** |", 1),
		want:  "zone table totals 4/2/1 but the rows are 3/2/1",
	}, {
		name:  "a per-zone row disagrees with its own rows",
		owner: strings.Replace(baselineValidOwnerBody, "| C 名稱 | 2 | 2 | 0 |", "| C 名稱 | 3 | 2 | 0 |", 1),
		want:  "zone C says 3/2/0 but has 2 rows",
	}, {
		name:  "the same id twice",
		owner: baselineValidOwnerBody + "| C-01 | desc | 阻擋 | note |\n",
		want:  "lists C-01 twice",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeBaseline(t, tc.owner)
			problems := strings.Join(baselineTallyProblems(root), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestBaselineTallyCatchesAStaleProseFigureWithinTheFuzzyWindow(t *testing.T) {
	root := writeBaseline(t, baselineValidOwnerBody, "", "現行基線 5 項，仍待覆核。\n")
	problems := strings.Join(baselineTallyProblems(root), "\n")
	if !strings.Contains(problems, "names 5 baseline items") {
		t.Fatalf("a stale prose figure within ±3 of the row count was not caught: %v", problems)
	}
}

const sec009Quoter = "tools/sec009/README.md"

func TestBaselineTallyReadsTheSec009Runbook(t *testing.T) {
	t.Parallel()
	at := -1
	for i, quoter := range baselineQuoters {
		if quoter == sec009Quoter {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("%s is not among the quoters; it states the baseline size in five places", sec009Quoter)
	}
	for _, tc := range []struct {
		name, prose string
	}{
		{"全數 after the count", "驗收的 5 項全數通過。\n"},
		{"覆蓋 after the count", "10 個測項與 5 項覆蓋，證據落檔。\n"},
		{"裡 after the count", "5 項裡有 2 項有證據。\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bodies := make([]string, len(baselineQuoters))
			bodies[at] = tc.prose
			problems := strings.Join(baselineTallyProblems(writeBaseline(t, baselineValidOwnerBody, bodies...)), "\n")
			want := sec009Quoter + ":1 names 5 baseline items"
			if !strings.Contains(problems, want) {
				t.Fatalf("want a problem containing %q, got %v", want, problems)
			}
		})
	}
}

func TestBaselineTallyCatchesTheEnglishPassShape(t *testing.T) {
	t.Parallel()
	root := writeBaseline(t, baselineValidOwnerBody, "", "`SEC-009` 十個測項實跑，5 項全 pass、0 unknown。\n")
	problems := strings.Join(baselineTallyProblems(root), "\n")
	if !strings.Contains(problems, "names 5 baseline items") {
		t.Fatalf("a gate threshold written as 「N 項全 pass」 was not caught: %v", problems)
	}
}

func TestBaselineTallyLeavesAFigureCountingSomethingElseAlone(t *testing.T) {
	t.Parallel()
	root := writeBaseline(t, baselineValidOwnerBody, "", "M4 開工時這 5 項全部是 `- [ ]`，沒有任何一項被退回。\n")
	if problems := baselineTallyProblems(root); len(problems) != 0 {
		t.Fatalf("a work-item count that never named the baseline was reported: %v", problems)
	}
}

func TestBaselineTallyReadsTheLiveGateDocuments(t *testing.T) {
	t.Parallel()
	for _, want := range []string{
		"docs/plans/04-backlog-and-handoffs.md",
		"docs/plans/mvp/m4/README.md",
		"docs/plans/mvp/m4/beta-design.md",
		"docs/plans/mvp/m4/release-checklist.md",
	} {
		found := false
		for _, quoter := range baselineQuoters {
			if quoter == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s states the gate threshold a person reads before opening the beta, and is not among the quoters", want)
		}
	}
}
