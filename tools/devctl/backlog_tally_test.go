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
