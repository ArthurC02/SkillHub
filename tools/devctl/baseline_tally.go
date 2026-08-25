package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The SEC-002 baseline states its own size in six places, and the rows are the
// only authority on it.
//
// a7e1699 added N-08 to §4.3 and touched none of them. The threat model then
// contradicted itself -- eight N rows, a zone table saying seven, a total
// saying 45 -- and `02:SEC-002` quoted the stale figure four times over.
//
// The arithmetic was the small half. ADR-022 §2's coverage cross-check assigns
// test items zone by zone, so a zone whose count is stale is a zone whose last
// rows have no test item, while §3's pass criterion counts to a total that does
// not include them. A check that is in the baseline and invisible to the
// acceptance table is worse than one that is absent, because the table reads as
// complete. Nothing compares the rows to the numbers, so this does.
//
// Same shape as milestone-tally, and for the same reason: a derived number with
// several authors.

const baselineOwner = "docs/plans/mvp/m0/threat-model-and-sandbox-baseline.md"

// Documents that state the baseline's current size and must agree with it.
//
// ADR-022 is deliberately absent. It names 45 in nine places and every one of
// them is the figure on its own decision date; AGENTS.md does not rewrite an
// accepted ADR's decision text, and its 2026-08-26 補記 carries the current
// reading instead. Listing it here would make this check demand exactly the
// edit the ADR discipline forbids.
var baselineQuoters = []string{
	"docs/plans/02-specifications-and-acceptance-criteria.md",
}

var (
	// A baseline row: `| C-01 | ... | 啟動前 | 阻擋 | ADR-005 |`. The level
	// column is what makes the blocking/warning split countable, and matching
	// the whole row rather than just the ID keeps prose references (`N-01～N-07`
	// in a sentence, `C-09（T8）` in a coverage table) from voting.
	baselineRow = regexp.MustCompile(`(?m)^\|\s*([CPNDIX])-(\d{2})\s*\|.*\|\s*(阻擋|告警)\s*\|[^|]*\|\s*$`)
	// The stated totals: `**合計：46 項檢查（阻擋 43 項、告警 3 項）。**`
	baselineTotal = regexp.MustCompile(`合計：(\d+)\s*項檢查（阻擋\s*(\d+)\s*項、告警\s*(\d+)\s*項）`)
	// The zone table's own row: `| C 計算隔離 | 16 | 16 | 0 |`
	baselineZoneRow = regexp.MustCompile(`(?m)^\|\s*([CPNDIX])\s[^|]*\|\s*(\d+)\s*\|\s*(\d+)\s*\|\s*(\d+)\s*\|\s*$`)
	// ...and its summary row: `| **合計** | **46** | **43** | **3** |`. Checked
	// separately because the zone pattern keys on the leading letter, so the
	// one row that carries no letter is the one row it cannot see -- and it is
	// the row a reader trusts.
	baselineZoneTotal = regexp.MustCompile(`(?m)^\|\s*\*\*合計\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*$`)
	// Any other sentence naming the size: `46 項（阻擋 43、告警 3）`,
	// `46 項全部歸屬`, `下表覆蓋全部 46 項`, `測試覆蓋基線 46 項的全部`,
	// `46 項基線尚未經`, `46 項全數 pass`.
	baselineProse = regexp.MustCompile(`(\d+)\s*項(?:檢查)?(?:全數|全部|全過|基線|的全部)|基線\s*(\d+)\s*項|覆蓋(?:核對)?（(\d+)\s*項）|(\d+)\s*項（阻擋`)
	// A sentence that names a date is describing that date. Every false
	// positive on the first run was one: `v2 的 32 條威脅與 45 項基線檢查`,
	// `2026-08-16 定案`, and the note added alongside N-08 explaining that 45
	// WAS the total until 2026-08-25. Recording what a figure used to be is how
	// a document stays honest about drift; a check that forbids it teaches
	// people to delete the history instead.
	baselineDated = regexp.MustCompile(`20\d\d-\d\d-\d\d|\bv[12]\b`)
)

// zoneName maps the letter to the label used in messages.
var zoneOrder = []string{"C", "P", "N", "D", "I", "X"}

func baselineTallyProblems(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(baselineOwner)))
	if err != nil {
		return []string{fmt.Sprintf("baseline-tally: cannot read %s: %v", baselineOwner, err)}
	}
	owner := string(raw)

	// Count the rows, which are the authority.
	seen := map[string]bool{}
	perZone := map[string][3]int{} // total, blocking, warning
	for _, m := range baselineRow.FindAllStringSubmatch(owner, -1) {
		id := m[1] + "-" + m[2]
		if seen[id] {
			return []string{fmt.Sprintf("baseline-tally: %s lists %s twice", baselineOwner, id)}
		}
		seen[id] = true
		c := perZone[m[1]]
		c[0]++
		if m[3] == "阻擋" {
			c[1]++
		} else {
			c[2]++
		}
		perZone[m[1]] = c
	}
	total, blocking, warning := 0, 0, 0
	for _, z := range zoneOrder {
		total += perZone[z][0]
		blocking += perZone[z][1]
		warning += perZone[z][2]
	}
	if total == 0 {
		return []string{fmt.Sprintf(
			"baseline-tally: %s has no `| X-NN | ... | 阻擋/告警 | ... |` rows; this check has lost its subject",
			baselineOwner)}
	}

	var problems []string

	// The stated total must be what the rows say.
	m := baselineTotal.FindStringSubmatch(owner)
	if m == nil {
		problems = append(problems, fmt.Sprintf(
			"baseline-tally: %s states no 「合計：N 項檢查（阻擋 N 項、告警 N 項）」; the rows say %d (%d/%d)",
			baselineOwner, total, blocking, warning))
	} else if atoi(m[1]) != total || atoi(m[2]) != blocking || atoi(m[3]) != warning {
		problems = append(problems, fmt.Sprintf(
			"baseline-tally: %s says 合計 %s 項（阻擋 %s、告警 %s） but its rows are %d（阻擋 %d、告警 %d）",
			baselineOwner, m[1], m[2], m[3], total, blocking, warning))
	}

	// The zone table's summary row must agree too.
	if zt := baselineZoneTotal.FindStringSubmatch(owner); zt != nil {
		if atoi(zt[1]) != total || atoi(zt[2]) != blocking || atoi(zt[3]) != warning {
			problems = append(problems, fmt.Sprintf(
				"baseline-tally: %s zone table totals %s/%s/%s but the rows are %d/%d/%d",
				baselineOwner, zt[1], zt[2], zt[3], total, blocking, warning))
		}
	}

	// Each zone row must be what that zone's rows say.
	for _, zm := range baselineZoneRow.FindAllStringSubmatch(owner, -1) {
		z := zm[1]
		c, ok := perZone[z]
		if !ok {
			continue
		}
		if atoi(zm[2]) != c[0] || atoi(zm[3]) != c[1] || atoi(zm[4]) != c[2] {
			problems = append(problems, fmt.Sprintf(
				"baseline-tally: %s zone %s says %s/%s/%s but has %d rows (%d 阻擋, %d 告警)",
				baselineOwner, z, zm[2], zm[3], zm[4], c[0], c[1], c[2]))
		}
	}

	// Any other sentence naming a baseline size, here and in the quoters, must
	// name this one. A stale figure in prose is exactly how N-08 stayed
	// invisible: the rows were right and every sentence about them was not.
	for _, path := range append([]string{baselineOwner}, baselineQuoters...) {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			problems = append(problems, fmt.Sprintf("baseline-tally: cannot read %s: %v", path, err))
			continue
		}
		problems = append(problems, staleBaselineFigures(path, string(body), total)...)
	}
	return problems
}

// staleBaselineFigures reports sentences that name a baseline size other than
// the current one.
//
// Only figures within striking distance of the real total count: the documents
// are full of unrelated numbers, and a check that flags `03` §16's ten work
// items would be turned off within a week. The window is what a drifting
// baseline actually looks like -- one or two rows added or removed at a time.
func staleBaselineFigures(path, body string, total int) []string {
	var problems []string
	for i, line := range strings.Split(body, "\n") {
		// The line stating the authoritative total is checked above, and a dated
		// sentence is history rather than drift.
		if baselineTotal.MatchString(line) || baselineDated.MatchString(line) {
			continue
		}
		for _, m := range baselineProse.FindAllStringSubmatch(line, -1) {
			for _, g := range m[1:] {
				if g == "" {
					continue
				}
				n := atoi(g)
				if n != total && n >= total-3 && n <= total+3 {
					problems = append(problems, fmt.Sprintf(
						"baseline-tally: %s:%d names %d baseline items; the rows in %s say %d",
						path, i+1, n, baselineOwner, total))
				}
			}
		}
	}
	return problems
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
