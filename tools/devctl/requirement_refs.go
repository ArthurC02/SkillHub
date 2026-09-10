package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const requirementSpec = "docs/plans/02-specifications-and-acceptance-criteria.md"

var requirementCiters = []string{
	"docs/plans/03-work-items.md",
	"docs/plans/04-backlog-and-handoffs.md",
	"docs/plans/05-pending-rulings.md",
}

var (
	requirementHeading = regexp.MustCompile(`^(#{2,6})\s+(.*)$`)

	requirementID = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,7}-\d{3}\b`)

	// Looser than requirementID on purpose: a malformed citation must still
	// match here so it gets reported, instead of silently matching nothing.
	requirementCitation = regexp.MustCompile(`02:([A-Za-z0-9]+-[0-9]+)`)
)

type headingOccurrence struct {
	depth, line int
}

func requirementRefProblems(root string) []string {
	headings, problems := specHeadingIDs(filepath.Join(root, filepath.FromSlash(requirementSpec)))
	if len(problems) > 0 {
		return problems
	}
	if len(headings) < 40 {
		return []string{fmt.Sprintf(
			"requirement-refs: %s declares %d heading ids; it has had more than forty since M2, so the "+
				"heading scan is broken rather than the spec emptied", requirementSpec, len(headings))}
	}

	var ids []string
	for id := range headings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		at := headings[id]
		if len(at) == 1 {
			continue
		}
		shallowest := at[0].depth
		for _, occurrence := range at[1:] {
			if occurrence.depth < shallowest {
				shallowest = occurrence.depth
			}
		}
		var siblings []int
		for _, occurrence := range at {
			if occurrence.depth == shallowest {
				siblings = append(siblings, occurrence.line)
			}
		}
		if len(siblings) > 1 {
			problems = append(problems, fmt.Sprintf(
				"requirement-refs: %s declares %s in %d headings at the same depth (lines %v); a `02:%s` "+
					"citation cannot say which one it means. One section owns an id; deeper sub-headings "+
					"may repeat it (the SEC-010 shape), same-depth ones may not",
				requirementSpec, id, len(siblings), siblings, id))
		}
	}

	var citations int
	for _, relative := range requirementCiters {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			problems = append(problems, fmt.Sprintf("requirement-refs: %v", err))
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range requirementCitation.FindAllStringSubmatch(line, -1) {
				citations++
				if len(headings[m[1]]) > 0 {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"requirement-refs: %s:%d cites `02:%s`, and %s has no heading declaring %s. "+
						"`02:<ID>` means \"the requirement with that heading in 02\"; if the number is "+
						"defined somewhere else (an m0 proposal, an 03 work item, a line range), cite it "+
						"the way that document is cited",
					relative, i+1, m[1], requirementSpec, m[1]))
			}
		}
	}
	if citations == 0 {
		problems = append(problems, fmt.Sprintf(
			"requirement-refs: not one `02:<ID>` citation was found in %s; there were around forty, so "+
				"the citation scan is broken rather than the citations removed",
			strings.Join(requirementCiters, ", ")))
	}
	sort.Strings(problems)
	return problems
}

func specHeadingIDs(path string) (map[string][]headingOccurrence, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("requirement-refs: %v", err)}
	}
	headings := map[string][]headingOccurrence{}
	for i, line := range strings.Split(string(data), "\n") {
		m := requirementHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, id := range requirementID.FindAllString(m[2], -1) {
			headings[id] = append(headings[id], headingOccurrence{depth: len(m[1]), line: i + 1})
		}
	}
	return headings, nil
}
