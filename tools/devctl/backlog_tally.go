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

const backlogDoc = "docs/plans/04-backlog-and-handoffs.md"

var (
	// A ledger row: "| category | **count** | ... |".
	backlogLedger = regexp.MustCompile(`^\|\s*(甲|乙|丙)\s*\|\s*\*\*(\d+)\*\*\s*\|`)

	// The trailer listing open ids, e.g. "<!-- open: 1,2,3 -->".
	backlogOpen = regexp.MustCompile(`<!--\s*open:\s*([^>]*?)\s*-->`)

	// Strips leading emphasis/strikethrough so a closed row still matches.
	backlogItem = regexp.MustCompile(`^\|\s*[~*]{0,4}(甲|乙|丙)-(\d+)`)
)

var closedMarkers = []string{"已結案", "入列即結案", "已解決", "✅"}

// A row declares its own closure only in its id cell or its last cell;
// everything between is prose that may discuss other rows' closures.
func backlogRowClosed(line string) bool {
	cells := strings.Split(line, "|")
	if len(cells) < 3 {
		return false
	}
	id, last := cells[1], cells[len(cells)-2]

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

	items := backlogItems(lines)
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
		problems = append(problems, backlogLedgerProblems(n+1, line, m, items)...)
	}
	if ledgers == 0 {
		problems = append(problems, fmt.Sprintf(
			"backlog-tally: %s has no `| 甲 | **n** |` ledger rows; this check has lost its subject",
			backlogDoc))
	}

	sort.Strings(problems)
	return problems
}

type backlogRow struct {
	line   int
	closed bool
}

func backlogItems(lines []string) map[string]backlogRow {
	items := map[string]backlogRow{}
	for n, line := range lines {
		m := backlogItem.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := m[1] + "-" + m[2]
		if _, seen := items[id]; !seen {
			items[id] = backlogRow{line: n + 1, closed: backlogRowClosed(line)}
		}
	}
	return items
}

func backlogLedgerProblems(lineNumber int, line string, ledger []string, items map[string]backlogRow) []string {
	category, stated := ledger[1], ledger[2]
	want, _ := strconv.Atoi(stated)

	trailer := backlogOpen.FindStringSubmatch(line)
	if trailer == nil {
		return []string{fmt.Sprintf(
			"backlog-tally: %s:%d the %s ledger states **%s** with no `<!-- open: … -->` list; "+
				"the number is then a claim about rows nothing compares it to",
			backlogDoc, lineNumber, category, stated)}
	}

	var problems []string
	ids, dupes := backlogIDs(category, trailer[1])
	if len(ids) != want {
		problems = append(problems, fmt.Sprintf(
			"backlog-tally: %s:%d the %s ledger states **%s** but lists %d: %s",
			backlogDoc, lineNumber, category, stated, len(ids), strings.Join(ids, ", ")))
	}
	for _, id := range dupes {
		problems = append(problems, fmt.Sprintf(
			"backlog-tally: %s:%d the %s ledger lists %s twice", backlogDoc, lineNumber, category, id))
	}
	for _, id := range ids {
		row, ok := items[id]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"backlog-tally: %s:%d the %s ledger lists %s, which is not a row in this file",
				backlogDoc, lineNumber, category, id))
			continue
		}
		if row.closed {
			problems = append(problems, fmt.Sprintf(
				"backlog-tally: %s:%d the %s ledger lists %s as open, but its row (line %d) "+
					"records it closed", backlogDoc, lineNumber, category, id, row.line))
		}
	}
	return problems
}

// Reads a comma-separated id list where an id may omit its category letter.
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
