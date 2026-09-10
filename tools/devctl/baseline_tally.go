package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const baselineOwner = "docs/plans/mvp/m0/threat-model-and-sandbox-baseline.md"

var baselineQuoters = []string{
	"docs/plans/02-specifications-and-acceptance-criteria.md",
	"docs/plans/03-work-items.md",
}

var (
	// A baseline row: "| X-00 | ... | 阻擋/告警 | ... |".
	baselineRow = regexp.MustCompile(`(?m)^\|\s*([CPNDIX])-(\d{2})\s*\|.*\|\s*(阻擋|告警)\s*\|[^|]*\|\s*$`)

	baselineTotal = regexp.MustCompile(`合計：(\d+)\s*項檢查（阻擋\s*(\d+)\s*項、告警\s*(\d+)\s*項）`)

	// A zone table row: "| X 名稱 | total | blocking | warning |".
	baselineZoneRow = regexp.MustCompile(`(?m)^\|\s*([CPNDIX])\s[^|]*\|\s*(\d+)\s*\|\s*(\d+)\s*\|\s*(\d+)\s*\|\s*$`)

	// The zone table's own summary row, matched separately since it carries
	// no leading zone letter.
	baselineZoneTotal = regexp.MustCompile(`(?m)^\|\s*\*\*合計\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*\*\*(\d+)\*\*\s*\|\s*$`)

	baselineProse = regexp.MustCompile(`(\d+)\s*項(?:檢查)?(?:全數|全部|全過|基線|的全部)|基線\s*(\d+)\s*項|覆蓋(?:核對)?（(\d+)\s*項）|(\d+)\s*項（阻擋`)

	// A sentence naming a date, which describes a past figure rather than
	// current drift.
	baselineDated = regexp.MustCompile(`20\d\d-\d\d-\d\d|\bv[12]\b`)
)

var zoneOrder = []string{"C", "P", "N", "D", "I", "X"}

func baselineTallyProblems(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(baselineOwner)))
	if err != nil {
		return []string{fmt.Sprintf("baseline-tally: cannot read %s: %v", baselineOwner, err)}
	}
	owner := string(raw)

	seen := map[string]bool{}
	perZone := map[string][3]int{} // [total, blocking, warning]
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

	if zt := baselineZoneTotal.FindStringSubmatch(owner); zt != nil {
		if atoi(zt[1]) != total || atoi(zt[2]) != blocking || atoi(zt[3]) != warning {
			problems = append(problems, fmt.Sprintf(
				"baseline-tally: %s zone table totals %s/%s/%s but the rows are %d/%d/%d",
				baselineOwner, zt[1], zt[2], zt[3], total, blocking, warning))
		}
	}

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

func staleBaselineFigures(path, body string, total int) []string {
	var problems []string
	for i, line := range strings.Split(body, "\n") {

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
