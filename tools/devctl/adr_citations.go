package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const adrIndex = "docs/adr/README.md"

const adrCitationCeiling = 852

const adrIndexRowFloor = 80

var (
	adrIndexRow  = regexp.MustCompile(`^\| \[(ADR-\d{3})\]\([^)]*\) \|(.*)\|([^|]*)\|\s*$`)
	adrID        = regexp.MustCompile(`ADR-\d{3}`)
	adrCitation  = regexp.MustCompile(`ADR-\d{3}(-)?`)
	adrFileName  = regexp.MustCompile(`^ADR-\d{3}-.*\.md$`)
	adrAmendLine = regexp.MustCompile(`^- (取代|修訂)：(.*)$`)
)

func adrCitationProblems(root string) []string {
	return adrCitationProblemsWithin(root, adrCitationCeiling)
}

func adrCitationProblemsWithin(root string, ceiling int) []string {
	index, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adrIndex)))
	if err != nil {
		return []string{fmt.Sprintf("adr-citations: %v", err)}
	}
	var problems []string
	rows := map[string]string{}
	successors := map[string][]string{}
	for _, line := range strings.Split(string(index), "\n") {
		m := adrIndexRow.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		id, status := m[1], strings.TrimSpace(m[3])
		rows[id] = m[2] + m[3]
		if !strings.HasPrefix(strings.TrimLeft(status, "*"), "Superseded") {
			continue
		}
		for _, successor := range uniqueADRs(status) {
			if successor != id {
				successors[id] = append(successors[id], successor)
			}
		}
		if len(successors[id]) == 0 {
			problems = append(problems, fmt.Sprintf(
				"adr-citations: %s marks %s Superseded without naming the ADR that superseded it; "+
					"write that ADR into the status cell, it is where a reader of the old one goes next",
				adrIndex, id))
		}
	}
	if len(rows) < adrIndexRowFloor {
		return append(problems, fmt.Sprintf(
			"adr-citations: %s has %d index rows; it has had more than %d, so the row scan is broken "+
				"rather than the index emptied", adrIndex, len(rows), adrIndexRowFloor))
	}

	problems = append(problems, adrAmendmentProblems(root, rows)...)

	files, err := adrCitationFiles(root)
	if err != nil {
		return append(problems, fmt.Sprintf("adr-citations: git ls-files: %v", err))
	}
	superseded := make([]string, 0, len(successors))
	for id := range successors {
		superseded = append(superseded, id)
	}
	sort.Strings(superseded)

	citations := 0
	for _, relative := range files {
		if adrHistory(relative) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil || !strings.Contains(string(body), "ADR-") {
			continue
		}
		text := string(body)
		for i, line := range strings.Split(text, "\n") {
			for _, old := range superseded {
				if strings.Contains(line, old) && !containsAnyOf(line, successors[old]) {
					problems = append(problems, fmt.Sprintf(
						"adr-citations: %s:%d cites %s, which %s says is superseded, without naming %s on the "+
							"same line. Cite the ADR that stands now; when the history is the point, name it beside the old one",
						relative, i+1, old, adrIndex, strings.Join(successors[old], " or ")))
				}
			}
		}
		if strings.HasSuffix(relative, ".md") || strings.HasSuffix(relative, ".jsonl") {
			continue
		}
		for _, m := range adrCitation.FindAllStringSubmatch(text, -1) {
			if m[1] == "" {
				citations++
			}
		}
	}

	switch {
	case citations > ceiling:
		problems = append(problems, fmt.Sprintf(
			"adr-citations: files other than markdown name an ADR number %d times, above the ceiling of %d. "+
				"Code, config, contracts and tests say the rule itself; the number belongs in the ADR, its index, "+
				"an AGENTS.md and the commit message (`git diff` shows which file gained one)", citations, ceiling))
	case citations < ceiling:
		problems = append(problems, fmt.Sprintf(
			"adr-citations: files other than markdown name an ADR number %d times, below the ceiling of %d; "+
				"lower adrCitationCeiling in tools/devctl/adr_citations.go to %d so the count cannot grow back",
			citations, ceiling, citations))
	}
	sort.Strings(problems)
	return problems
}

func adrAmendmentProblems(root string, rows map[string]string) []string {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("adr-citations: %v", err)}
	}
	var problems []string
	for _, entry := range entries {
		if !adrFileName.MatchString(entry.Name()) {
			continue
		}
		id := entry.Name()[:7]
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return append(problems, fmt.Sprintf("adr-citations: %v", err))
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "## ") {
				break
			}
			m := adrAmendLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			// A header line names what it amends before its first （ or ；;
			// what follows is commentary, often naming ADRs it merely agrees with.
			amended := m[2]
			if cut := strings.IndexAny(amended, "（；"); cut >= 0 {
				amended = amended[:cut]
			}
			for _, old := range uniqueADRs(amended) {
				row, indexed := rows[old]
				if old >= id || !indexed || strings.Contains(row, id) {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"adr-citations: %s says 「%s」 %s, and the %s row of %s does not name %s. Add it to that "+
						"row's status, e.g. `Accepted（決策 N 經 %s %s）`: the old record stays as written, the index "+
						"is where its reader learns it no longer stands alone",
					entry.Name(), m[1], old, adrIndex, old, id, id, m[1]))
			}
		}
	}
	return problems
}

func adrCitationFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, relative := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if relative == "" || strings.HasPrefix(relative, docLinkFrozenCorpus+"/") {
			continue
		}
		if strings.Contains("/"+relative, "/gen/") || strings.Contains("/"+relative, "/generated/") {
			continue
		}
		files = append(files, relative)
	}
	return files, nil
}

func adrHistory(relative string) bool {
	return strings.HasPrefix(relative, "docs/adr/") || strings.HasPrefix(relative, "docs/plans/mvp/")
}

func uniqueADRs(text string) []string {
	var ids []string
	for _, id := range adrID.FindAllString(text, -1) {
		if !contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func containsAnyOf(line string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
