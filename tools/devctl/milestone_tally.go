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

var (
	// A tally in prose: "9 勾", "9 項已勾", "2 項 ◐", "11 項中 9", and — the
	// shape M4's satellite uses — "十項全部不勾" in Chinese numerals. The
	// satellites are checked against this; the owner is checked against the
	// counts.
	tallyInProse = regexp.MustCompile(`[\d一二三四五六七八九十]+\s*(?:項)?\s*(?:全部)?\s*(?:已勾|不勾|勾)|\d+\s*項\s*◐`)
)

// A milestone whose ticked/unticked count is derived from checkboxes `03` owns.
//
// Two entries, and the second exists because the audit of 2026-08-29 found the
// M5 failure repeating verbatim one milestone earlier: `01` §10 says
// 「`RELEASE-001`～`010` **十項全部不勾**」 while `03` §18 has ticked two of them
// since 2026-08-28. A reader is making封測 decisions off ten red lights, two of
// which stopped being red ten days ago.
//
// The comment above records why M4's OTHER number (「49 項中 16 勾」) is left
// alone: those 49 items are a set defined in prose in m4/audit.md, so no machine
// can confirm or deny it, and flagging what cannot be checked is how a check
// loses its readers. RELEASE-001～010 is the opposite case — ten literal
// checkboxes — so the same reasoning that excuses the 49 obliges this.
type tallySubject struct {
	// prefix is the work-item id prefix whose checkboxes are the tally.
	prefix string
	// what names the milestone in the messages.
	what string
	// ownerSentence renders the sentence the owner's own section header must
	// carry, or nil when the owner states no count. RELEASE has none: `03` §18
	// carries a narrative of which items were re-judged and when, not a count,
	// and inventing a count sentence for it would put a second derived number in
	// the document that is supposed to be the only author of the first.
	ownerSentence func(ticked, open int) string
	// satellites may describe the milestone but may not state its count.
	satellites []string
	// nearby is what has to appear within a few lines of a prose count for it to
	// be about THIS milestone. Without it every count in a long document votes.
	nearby []string
}

func tallySubjects() []tallySubject {
	return []tallySubject{{
		prefix: "GEN-",
		what:   "M5",
		ownerSentence: func(ticked, open int) string {
			return fmt.Sprintf("%d 項已勾、%d 項 ◐", ticked, open)
		},
		satellites: []string{
			"AGENTS.md",
			"docs/plans/01-goals-and-plan.md",
			"docs/plans/mvp/README.md",
			"docs/plans/mvp/m5/README.md",
		},
		nearby: []string{"GEN-", "§19", "M5"},
	}, {
		prefix:     "RELEASE-",
		what:       "M4 的封測准入（RELEASE-001～010）",
		satellites: []string{"docs/plans/01-goals-and-plan.md"},
		nearby:     []string{"RELEASE-", "§18"},
	}}
}

func milestoneTallyProblems(root string) []string {
	owner, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tallyOwner)))
	if err != nil {
		return []string{fmt.Sprintf("milestone-tally: cannot read %s: %v", tallyOwner, err)}
	}
	var problems []string
	for _, subject := range tallySubjects() {
		problems = append(problems, subject.problems(root, string(owner))...)
	}
	return problems
}

func (s tallySubject) problems(root, owner string) []string {
	checked := regexp.MustCompile(`(?m)^- \[x\] ` + s.prefix)
	unchecked := regexp.MustCompile(`(?m)^- \[ \] ` + s.prefix)
	ticked := len(checked.FindAllString(owner, -1))
	open := len(unchecked.FindAllString(owner, -1))
	if ticked+open == 0 {
		return []string{fmt.Sprintf(
			"milestone-tally: %s has no `- [x] %s` / `- [ ] %s` items; this check has lost %s's subject",
			tallyOwner, s.prefix, s.prefix, s.what)}
	}

	var problems []string
	// The owner must state what its own boxes say.
	if s.ownerSentence != nil {
		want := s.ownerSentence(ticked, open)
		if !strings.Contains(owner, want) {
			problems = append(problems, fmt.Sprintf(
				"milestone-tally: %s has %d ticked and %d open %s items, so its section header must say %q and does not",
				tallyOwner, ticked, open, s.prefix, want))
		}
	}

	// Nobody else may state it at all.
	for _, rel := range s.satellites {
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
			relevant := false
			for _, needle := range s.nearby {
				if strings.Contains(near, needle) {
					relevant = true
					break
				}
			}
			if !relevant {
				continue
			}
			hit := text[loc[0]:loc[1]]
			problems = append(problems, fmt.Sprintf(
				"milestone-tally: %s states %s's tally (%q) while %s counts %d ticked and %d open. "+
					"Only %s may state the number — carry the narrative (which items and why) and point at it, "+
					"so the count has one author",
				rel, s.what, strings.TrimSpace(hit), tallyOwner, ticked, open, tallyOwner))
		}
	}
	return problems
}
