package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// One place may state the M5 tally, and it must match the checkboxes.
//
// The number "9 ticked, 2 at ◐" was written in five documents. Three of the six
// adversarial review rounds found it disagreeing with itself — 8/3 in one file
// while the items said 9/2, and a satellite still saying 8/3 a day after the
// other four were corrected. Nothing was wrong with any single edit; what was
// wrong is that a derived number had five authors.
//
// So: `03` §19's checkboxes ARE the tally, its own header sentence is checked
// against them, and the satellites may carry the narrative (which items are ◐
// and why — that is the useful half) but not the count. The rule is mechanical
// in both directions: the one copy must be right, and the others must not exist.
//
// Same shape as one-number, and for the same reason — a value that has to be
// identical in places no compiler compares.

// The document that owns the tally, and the section that holds the checkboxes.
const tallyOwner = "docs/plans/03-work-items.md"

// Documents that may describe M5's status but may not state its tally.
var tallySatellites = []string{
	"AGENTS.md",
	"docs/plans/01-goals-and-plan.md",
	"docs/plans/mvp/README.md",
	"docs/plans/mvp/m5/README.md",
}

var (
	genChecked   = regexp.MustCompile(`(?m)^- \[x\] GEN-`)
	genUnchecked = regexp.MustCompile(`(?m)^- \[ \] GEN-`)
	// A tally in prose: "9 勾", "9 項已勾", "2 項 ◐", "11 項中 9". The satellites
	// are checked against this; the owner is checked against the counts.
	tallyInProse = regexp.MustCompile(`\d+\s*(?:項)?\s*(?:已勾|勾)|\d+\s*項\s*◐`)
)

func milestoneTallyProblems(root string) []string {
	owner, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tallyOwner)))
	if err != nil {
		return []string{fmt.Sprintf("milestone-tally: cannot read %s: %v", tallyOwner, err)}
	}
	ticked := len(genChecked.FindAllString(string(owner), -1))
	open := len(genUnchecked.FindAllString(string(owner), -1))
	if ticked+open == 0 {
		return []string{fmt.Sprintf(
			"milestone-tally: %s has no `- [x] GEN-` / `- [ ] GEN-` items; this check has lost its subject",
			tallyOwner)}
	}

	var problems []string
	// The owner must state what its own boxes say.
	want := fmt.Sprintf("%d 項已勾、%d 項 ◐", ticked, open)
	if !strings.Contains(string(owner), want) {
		problems = append(problems, fmt.Sprintf(
			"milestone-tally: %s has %d ticked and %d open GEN items, so its §19 header must say %q and does not",
			tallyOwner, ticked, open, want))
	}

	// Nobody else may state it at all.
	//
	// Only M5's tally: a prose count is flagged when its neighbourhood names the
	// section it is derived from. 01's M4 line ("49 項中 16 勾") has the same
	// shape and the same risk, but its 49 items are a set defined in prose in
	// m4/audit.md, so no machine can confirm or deny it — flagging what cannot
	// be checked is how a check loses its readers.
	for _, rel := range tallySatellites {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		text := string(body)
		for _, loc := range tallyInProse.FindAllStringIndex(text, -1) {
			lo := loc[0] - 240
			if lo < 0 {
				lo = 0
			}
			hi := loc[1] + 240
			if hi > len(text) {
				hi = len(text)
			}
			near := text[lo:hi]
			if !strings.Contains(near, "GEN-") && !strings.Contains(near, "§19") && !strings.Contains(near, "M5") {
				continue
			}
			hit := text[loc[0]:loc[1]]
			problems = append(problems, fmt.Sprintf(
				"milestone-tally: %s states M5's tally (%q). Only %s may — say which items are ◐ and link to it, "+
					"so the number has one author",
				rel, strings.TrimSpace(hit), tallyOwner))
		}
	}
	return problems
}
