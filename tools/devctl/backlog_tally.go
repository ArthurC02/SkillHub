package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The 04 backlog's three counts, checked against the rows they claim to count.
//
// AGENTS.md names `04` as the single source of truth for how many items are
// still open, and the file says its counts are "逐列重數，不是把歷次加減累積出
// 來的". Nobody could verify that: the number and the rows it summarises had no
// relationship any tool could check, and it has disagreed with itself three
// times.
//
// Rather than force a status column onto rows written over three months, this
// takes the smallest thing that kills the recurring failure: each ledger cell
// carries a machine-readable list of the ids it says are open, and that list is
// checked three ways —
//
//	the count in the cell equals the length of the list
//	every id on the list is a row that exists
//	no id on the list is a row that declares itself closed
//
// The third one used to be decorative. It read cells[len-2] as "the status
// cell", and of this file's eight item sub-tables exactly one ends in a status
// column (`| # | 接點 | 內容 | M3 結案 |`); the others end in 解除條件,
// 阻擋什麼, 觸發／解除條件, 解鎖 or 依據. For most rows it was testing a link
// or a rationale for the word 已結案, so a listed-but-closed row went through.
// Closure is now read from the two slots this document really uses — see
// backlogRowClosed.
//
// What it deliberately does NOT check:
//
//   - whether a row absent from the list is really closed. That needs a per-row
//     status this document does not have, and assuming "unlisted means closed"
//     would assert some fifty closures nobody verified.
//   - a closure written only in a row's prose. 丙-8 and its neighbours record
//     theirs mid-paragraph, and those same paragraphs discuss other rows'
//     closures at length, so there is no reading of the prose that is not also a
//     false alarm on a row that is genuinely open.
//
// Both gaps are false negatives: this can miss a stale row, it cannot invent
// one. That is the right direction for a check nobody will re-derive.
//
// Same shape and same reason as one-number and milestone-tally: a fact with more
// than one author and no compiler between them.

const backlogDoc = "docs/plans/04-backlog-and-handoffs.md"

var (
	// A ledger row: `| 丙 | **7** | ...prose... <!-- open: 丙-26,38,... --> |`
	backlogLedger = regexp.MustCompile(`^\|\s*(甲|乙|丙)\s*\|\s*\*\*(\d+)\*\*\s*\|`)
	// The trailer. Ids may be written 丙-26 or bare 26 inside one list.
	backlogOpen = regexp.MustCompile(`<!--\s*open:\s*([^>]*?)\s*-->`)
	// An item row. Its id cell carries the row's own bookkeeping around the id —
	// `**丙-56（新入列…）**` while open, `~~丙-60~~ **已結案 2026-08-25**` once
	// retired — so both emphasis and strike-through are stepped over to reach it.
	// A struck row is still a row: leaving it unmatched made check 2 report a
	// listed id as "not a row in this file" when the truth is that it closed.
	backlogItem = regexp.MustCompile(`^\|\s*[~*]{0,4}(甲|乙|丙)-(\d+)`)
)

// closedMarkers are what this document writes when an item is done.
var closedMarkers = []string{"已結案", "入列即結案", "已解決", "✅"}

// backlogRowClosed reports whether an item row declares its own closure.
//
// It reads two slots and no others. The id cell, which this file strikes through
// and annotates in place when a row retires (`~~丙-60~~ **已結案 2026-08-25**`,
// `丙-64（新入列 2026-08-25，入列即結案）`), and the last cell, which the older
// tables use as a status column even where their header calls it 依據 (丙-50,
// 51, 54, 55 and 57 all write 已結案 there).
//
// Everything between them is prose, and prose is where rows discuss *other*
// rows' closures. 甲-1, 乙-9, 丙-52 and 丙-61 are all open today and all four say
// 已結案 or 已解決 mid-paragraph — 丙-52 does it while its own status cell reads
// 未結案. Scanning the whole row would fail this document for being well
// annotated, which is why the wider read was not taken.
func backlogRowClosed(line string) bool {
	cells := strings.Split(line, "|")
	if len(cells) < 3 {
		return false
	}
	id, last := cells[1], cells[len(cells)-2]
	// Struck through in place: this document's own way of retiring a row
	// (`維護方式`: 一項解除就就地標「已解決」…不刪除).
	if strings.HasPrefix(strings.TrimSpace(id), "~~") {
		return true
	}
	for _, marker := range closedMarkers {
		if strings.Contains(id, marker) || strings.Contains(last, marker) {
			return true
		}
	}
	return false
}

func backlogTallyProblems(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(backlogDoc)))
	if err != nil {
		return []string{fmt.Sprintf("backlog-tally: cannot read %s: %v", backlogDoc, err)}
	}
	lines := strings.Split(string(raw), "\n")

	// Every item row, and whether it declares its own closure.
	type item struct {
		line   int
		closed bool
	}
	items := map[string]item{}
	for n, line := range lines {
		m := backlogItem.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		closed := backlogRowClosed(line)
		id := m[1] + "-" + m[2]
		// A row written twice is its own problem; keep the first and let the
		// duplicate be reported below rather than silently overwriting.
		if _, seen := items[id]; !seen {
			items[id] = item{line: n + 1, closed: closed}
		}
	}
	if len(items) == 0 {
		return []string{fmt.Sprintf(
			"backlog-tally: %s has no 甲/乙/丙 rows; this check has lost its subject", backlogDoc)}
	}

	var problems []string
	ledgers := 0
	for n, line := range lines {
		m := backlogLedger.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ledgers++
		category, stated := m[1], m[2]
		want, _ := strconv.Atoi(stated)

		trailer := backlogOpen.FindStringSubmatch(line)
		if trailer == nil {
			problems = append(problems, fmt.Sprintf(
				"backlog-tally: %s:%d the %s ledger states **%s** with no `<!-- open: … -->` list; "+
					"the number is then a claim about rows nothing compares it to",
				backlogDoc, n+1, category, stated))
			continue
		}

		ids, dupes := backlogIDs(category, trailer[1])
		if len(ids) != want {
			problems = append(problems, fmt.Sprintf(
				"backlog-tally: %s:%d the %s ledger states **%s** but lists %d: %s",
				backlogDoc, n+1, category, stated, len(ids), strings.Join(ids, ", ")))
		}
		for _, id := range dupes {
			problems = append(problems, fmt.Sprintf(
				"backlog-tally: %s:%d the %s ledger lists %s twice", backlogDoc, n+1, category, id))
		}
		for _, id := range ids {
			row, ok := items[id]
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"backlog-tally: %s:%d the %s ledger lists %s, which is not a row in this file",
					backlogDoc, n+1, category, id))
				continue
			}
			if row.closed {
				problems = append(problems, fmt.Sprintf(
					"backlog-tally: %s:%d the %s ledger lists %s as open, but its row (line %d) "+
						"records it closed", backlogDoc, n+1, category, id, row.line))
			}
		}
	}
	if ledgers == 0 {
		problems = append(problems, fmt.Sprintf(
			"backlog-tally: %s has no `| 甲 | **n** |` ledger rows; this check has lost its subject",
			backlogDoc))
	}

	sort.Strings(problems)
	return problems
}

// backlogIDs reads the trailer's comma-separated ids, accepting both `丙-26` and
// a bare `26` under the row's own category, and reports any listed twice.
func backlogIDs(category, list string) (ids []string, dupes []string) {
	seen := map[string]bool{}
	for _, field := range strings.Split(list, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id := field
		if !strings.Contains(field, "-") {
			id = category + "-" + field
		}
		if seen[id] {
			dupes = append(dupes, id)
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, dupes
}
