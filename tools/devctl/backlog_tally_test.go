package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

		name:   "count disagrees with the list",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 2,3 --> |",
		want:   "states **3** but lists 2",
	}, {

		name:   "an id with no row",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 2,3,9 --> |",
		want:   "which is not a row in this file",
	}, {

		name:   "an item that closed itself",
		ledger: "| 丙 | **3** | 逐列重數 <!-- open: 1,2,3 --> |",
		want:   "records it closed",
	}, {

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

func TestBacklogTallyAcceptsTheRealDocument(t *testing.T) {

	if problems := backlogTallyProblems("../.."); len(problems) != 0 {
		t.Fatalf("the real %s was rejected:\n%s", backlogDoc, strings.Join(problems, "\n"))
	}
}

func TestBacklogTallyCatchesAClosedRowStillCountedAsOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{{

		name: "入列即結案 in the id cell",
		row:  "| **丙-2（新入列 2026-08-25，入列即結案）** | a | 補法的散文，沒有狀態欄 |",
	}, {

		name: "struck through with 已結案 in the id cell",
		row:  "| ~~**丙-2（新入列 2026-08-25）**~~ **已結案 2026-08-25** | a | 補法的散文 |",
	}, {

		name: "struck through and nothing else",
		row:  "| ~~丙-2~~ | a | b | 解鎖 |",
	}, {

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

func TestBacklogTallyDoesNotCloseARowForDiscussingAnotherRowsClosure(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{{

		name: "prose says 已解決 while the status cell says 未結案",
		row:  "| **丙-2** | **IA 盤點八項** | 其中四項已解決，IA-6 另立丙-9 | 未結案 |",
	}, {

		name: "prose reports a sibling's 已結案",
		row:  "| **丙-2** | **重掃從未跑完** | 丙-60 已結案（gVisor 腿已綠），但這一項不是它 | 一次綠的排程 |",
	}, {

		name: "a ✅ inside the narrative",
		row:  "| **丙-2** | **部署期驗收** | ✅ 契約已補；仍等節點 | 解除條件見 m4/release-checklist.md |",
	}, {

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
