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
// 來的". Nobody could verify that. Of 56 丙 rows only 13 record a closure in a
// cell a machine can read; the rest record it in prose, in a paragraph, inside
// the row — so the number and the rows it summarises had no relationship any
// tool could check, and the number has disagreed with itself three times.
//
// Rather than force a status column onto 67 rows written over two months, this
// takes the smallest thing that kills the recurring failure: each ledger cell
// carries a machine-readable list of the ids it says are open, and that list is
// checked three ways —
//
//	the count in the cell equals the length of the list
//	every id on the list is a row that exists
//	no id on the list is a row whose own status cell says it closed
//
// What it deliberately does NOT check: whether a row absent from the list is
// really closed. That needs a per-row status this document does not have, and
// inventing one by assuming "unlisted means closed" would assert forty-nine
// closures nobody verified. The gap is real and is better named than papered
// over: this catches the number drifting from its own enumeration, which is the
// failure that actually happened, not every failure imaginable.
//
// Same shape and same reason as one-number and milestone-tally: a fact with more
// than one author and no compiler between them.

const backlogDoc = "docs/plans/04-backlog-and-handoffs.md"

var (
	// A ledger row: `| 丙 | **7** | ...prose... <!-- open: 丙-26,38,... --> |`
	backlogLedger = regexp.MustCompile(`^\|\s*(甲|乙|丙)\s*\|\s*\*\*(\d+)\*\*\s*\|`)
	// The trailer. Ids may be written 丙-26 or bare 26 inside one list.
	backlogOpen = regexp.MustCompile(`<!--\s*open:\s*([^>]*?)\s*-->`)
	// An item row: `| **丙-56（新入列…）** | … | … | 已結案 |`
	backlogItem = regexp.MustCompile(`^\|\s*\*{0,2}(甲|乙|丙)-(\d+)`)
)

// closedMarkers are what an item row's last cell says when the item is done.
var closedMarkers = []string{"已結案", "✅"}

func backlogTallyProblems(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(backlogDoc)))
	if err != nil {
		return []string{fmt.Sprintf("backlog-tally: cannot read %s: %v", backlogDoc, err)}
	}
	lines := strings.Split(string(raw), "\n")

	// Every item row, and whether its status cell records a closure.
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
		cells := strings.Split(line, "|")
		status := ""
		if len(cells) >= 2 {
			status = cells[len(cells)-2]
		}
		closed := false
		for _, marker := range closedMarkers {
			if strings.Contains(status, marker) {
				closed = true
			}
		}
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
