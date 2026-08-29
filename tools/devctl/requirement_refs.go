package main

// `02:<ID>` means "the requirement with that heading in 02". Nothing checked
// that the heading existed.
//
// doc_identifiers.go already asks "does this identifier exist in the code". The
// gap it leaves is the other half of the same question: `03`, `04` and `05` cite
// `02` roughly forty times, and a citation that resolves to nothing looks
// exactly like one that resolves. Three of them do not resolve today, and each
// is a different way of being wrong:
//
//	02:PDM-005    PDM-005 is a PROPOSAL number in mvp/m0/pdm-proposals.md. `02`
//	              quotes it; `02` does not define it. The `02:` prefix says
//	              otherwise, and a reader who goes looking finds nothing.
//	02:SBX-008    the same shape with an `03` work-item id.
//	02:736-759    a LINE RANGE written in requirement-id notation.
//
// None of the three is a lie about the substance; each is a lie about where to
// look, which is the only thing the notation is for.
//
// The second half is uniqueness, because a reference is only unambiguous while
// the target is. `02` has exactly one repeated id and it is a legitimate shape:
// `### SEC-010：…` with `#### SEC-010 事件嚴重度分級與回應` nested under it. THE
// RULE, decided here and stated so the next repeat has to argue with it: an id
// may appear in more than one heading only when exactly one occurrence is the
// shallowest and every other is strictly deeper — a sub-section of the section
// that owns the id. Two `###` headings with the same id is two definitions, and
// that fails.
//
// WHAT THIS DOES NOT COVER, deliberately. `02` also cites requirement ids in
// prose without the `02:` prefix, and one of those is dangling too
// (`DISC-005` at 02:616 — search's no-result state, which `02` never defines).
// A rule broad enough to catch it flags four sentences that write `` `03` ``
// followed by an `03` id, which is correct notation. Flagging what is not wrong
// is how a check loses its readers, so that one is left to a human and recorded
// here rather than half-enforced.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const requirementSpec = "docs/plans/02-specifications-and-acceptance-criteria.md"

// The live documents that cite the spec.
var requirementCiters = []string{
	"docs/plans/03-work-items.md",
	"docs/plans/04-backlog-and-handoffs.md",
	"docs/plans/05-pending-rulings.md",
}

var (
	// A heading, with its depth. `~~PORT-002：…~~（撤回）` still declares the id:
	// a withdrawn requirement is a place a citation may legitimately land, and
	// the strikethrough is the narrative, not the definition.
	requirementHeading = regexp.MustCompile(`^(#{2,6})\s+(.*)$`)
	// A requirement id inside a heading. The prefix must start with a letter so
	// that a section number cannot become an id.
	requirementID = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,7}-\d{3}\b`)
	// A citation. Deliberately looser than requirementID on both halves so that
	// `02:736-759` is REPORTED rather than silently unmatched — a malformed
	// citation and a missing target are the same defect to a reader.
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

	// Uniqueness first: an ambiguous target makes every citation to it moot.
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
