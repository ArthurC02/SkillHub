package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A backlog file in the shape the real one has: a ledger table whose cells carry
// an `<!-- open: … -->` trailer, and item rows whose last cell may record a
// closure.
func writeBacklog(t *testing.T, ledger, items string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "| 類 | 未結案 | 內容 |\n| --- | --- | --- |\n" + ledger + "\n\n" + items + "\n"
	if err := os.WriteFile(filepath.Join(dir, "04-backlog-and-handoffs.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

const twoOpenItems = "" +
	"| **丙-1** | a | b | 已結案 |\n" +
	"| **丙-2** | a | b | 等部署 |\n" +
	"| **丙-3** | a | b | 等封測流量 |\n"

func TestBacklogTallyAcceptsALedgerThatMatchesItsRows(t *testing.T) {
	root := writeBacklog(t,
		"| 丙 | **2** | 逐列重數 <!-- open: 2,3 --> |",
		twoOpenItems)
	if problems := backlogTallyProblems(root); len(problems) != 0 {
		t.Fatalf("a correct ledger was rejected: %v", problems)
	}
}

func TestBacklogTallyRejectsTheThreeWaysTheNumberHasDrifted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ledger string
		want   string
	}{{
		// The failure that actually happened, three times: the count and the
		// enumeration in the same cell disagree.
		name:   "count disagrees with the list",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 2,3 --> |",
		want:   "states **3** but lists 2",
	}, {
		// A reference to a row that does not exist - the shape a code comment
		// citing 04 丙-57 had while no such row was written.
		name:   "an id with no row",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 2,3,9 --> |",
		want:   "which is not a row in this file",
	}, {
		// The one direction a human eye slides over: an item closed in its own
		// row and still counted as open.
		name:   "an item that closed itself",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 1,2,3 --> |",
		want:   "records it closed",
	}, {
		// Without a trailer the number is unverifiable, which is the state this
		// check was written to end rather than to tolerate.
		name:   "no list at all",
		ledger: "| 丙 | **2** | 逐列重數 |",
		want:   "with no `<!-- open: … -->` list",
	}, {
		name:   "the same id twice",
		ledger: "| 丙 | **2** | 逐列重數 <!-- open: 2,2 --> |",
		want:   "lists 丙-2 twice",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeBacklog(t, tc.ledger, twoOpenItems)
			problems := backlogTallyProblems(root)
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// A check whose subject disappears has to say so rather than pass. Both of these
// are green under a naive implementation, and a silently-green checker is worse
// than none: it is the thing the next person trusts.
func TestBacklogTallySaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Run("no item rows", func(t *testing.T) {
		root := writeBacklog(t, "| 丙 | **2** | 逐列重數 <!-- open: 2,3 --> |", "")
		if problems := backlogTallyProblems(root); len(problems) == 0 {
			t.Fatal("a file with no item rows was accepted")
		}
	})
	t.Run("no ledger rows", func(t *testing.T) {
		root := writeBacklog(t, "", twoOpenItems)
		if problems := backlogTallyProblems(root); len(problems) == 0 {
			t.Fatal("a file with no ledger rows was accepted")
		}
	})
	t.Run("no file", func(t *testing.T) {
		if problems := backlogTallyProblems(t.TempDir()); len(problems) == 0 {
			t.Fatal("a missing backlog file was accepted")
		}
	})
}

// The real 04, which every one of these rules has to survive. Its rows are
// written eight different ways across eight sub-tables, and four rows that are
// open right now (甲-1, 乙-9, 丙-52, 丙-61) say 已結案 or 已解決 somewhere in
// their prose. A closure rule that reads more of a row than it should fails
// here, not in a fixture.
func TestBacklogTallyAcceptsTheRealDocument(t *testing.T) {
	// ../.. is the repo root, the same anchor retention_floor_test.go uses.
	if problems := backlogTallyProblems("../.."); len(problems) != 0 {
		t.Fatalf("the real %s was rejected:\n%s", backlogDoc, strings.Join(problems, "\n"))
	}
}

// Check 3 with the teeth it did not have. Each of these is a row shape the real
// file uses to say "closed" while the ledger still counts it as open - the one
// direction a human eye slides over, and the one the positional `cells[len-2]`
// read could not see for seven of the eight sub-tables.
func TestBacklogTallyCatchesAClosedRowStillCountedAsOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{{
		// 丙-64/70/71/72's shape: the closure is in the id cell and the last
		// cell is 補法 prose. Under the old positional read this was invisible.
		name: "入列即結案 in the id cell",
		row:  "| **丙-2（新入列 2026-08-25，入列即結案）** | a | 補法的散文，沒有狀態欄 |",
	}, {
		// 丙-60's shape: struck through and annotated in place.
		name: "struck through with 已結案 in the id cell",
		row:  "| ~~**丙-2（新入列 2026-08-25）**~~ **已結案 2026-08-25** | a | 補法的散文 |",
	}, {
		// A row struck through and nothing else: this file retires rows in
		// place rather than deleting them, so the strike is the statement.
		name: "struck through and nothing else",
		row:  "| ~~丙-2~~ | a | b | 解鎖 |",
	}, {
		// The one shape the old read did catch, kept so the fix does not
		// quietly drop it.
		name: "已結案 in a status-shaped last cell",
		row:  "| **丙-2** | a | b | 已結案 |",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			items := "| **丙-1** | a | b | 已結案 |\n" + tc.row + "\n| **丙-3** | a | b | 等部署 |\n"
			root := writeBacklog(t, "| 丙 | **2** | 逐列重數 <!-- open: 2,3 --> |", items)
			problems := strings.Join(backlogTallyProblems(root), "\n")
			if !strings.Contains(problems, "lists 丙-2 as open, but its row") {
				t.Fatalf("a closed row counted as open was accepted; got: %v", problems)
			}
		})
	}
}

// The other half of the same rule, and the reason the row's prose is not read.
// This document argues with itself in the middle cells: an open row routinely
// explains which OTHER rows closed, and 丙-52 does it while its own status cell
// reads 未結案. Every row here is genuinely open and must stay that way.
func TestBacklogTallyDoesNotCloseARowForDiscussingAnotherRowsClosure(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{{
		// 丙-52's shape, verbatim in structure: 已解決 in the prose, 未結案 in
		// the status cell.
		name: "prose says 已解決 while the status cell says 未結案",
		row:  "| **丙-2** | **IA 盤點八項** | 其中四項已解決，IA-6 另立丙-9 | 未結案 |",
	}, {
		// 丙-61's shape: the row explains what a sibling's closure did not fix.
		name: "prose reports a sibling's 已結案",
		row:  "| **丙-2** | **重掃從未跑完** | 丙-60 已結案（gVisor 腿已綠），但這一項不是它 | 一次綠的排程 |",
	}, {
		// 甲-1 and 乙-9's shape: a ✅ inside the narrative, no status column at
		// all in this sub-table.
		name: "a ✅ inside the narrative",
		row:  "| **丙-2** | **部署期驗收** | ✅ 契約已補；仍等節點 | 解除條件見 m4/release-checklist.md |",
	}, {
		// The false positive the main agent's new rows are shaped like: the
		// word 結案 appears, 已結案 does not.
		name: "the bare word 結案 in a dependency cell",
		row:  "| **丙-2** | **生成前的預估成本** | 沒有可顯示的數字 | `GEN-008` 結案 |",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			items := "| **丙-1** | a | b | 已結案 |\n" + tc.row + "\n| **丙-3** | a | b | 等部署 |\n"
			root := writeBacklog(t, "| 丙 | **2** | 逐列重數 <!-- open: 2,3 --> |", items)
			if problems := backlogTallyProblems(root); len(problems) != 0 {
				t.Fatalf("an open row was called closed for describing another row: %v", problems)
			}
		})
	}
}
