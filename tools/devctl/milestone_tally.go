package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const tallyOwner = "docs/plans/03-work-items.md"

var (
	// Matches a checkbox tally in prose: "9 勾", "9 項已勾", "2 項 ◐", or a
	// Chinese-numeral phrasing like "十項全部不勾".
	tallyInProse = regexp.MustCompile(`[\d一二三四五六七八九十]+\s*(?:項)?\s*(?:全部)?\s*(?:已勾|不勾|勾)|\d+\s*項\s*◐`)

	// A different phrasing that names states rather than checkboxes, e.g.
	// "完成七項、撤回兩項、剩兩項".
	portTallyInProse = regexp.MustCompile(`(?:完成|撤回|剩下|剩)\s*[\d一二三四五六七八九十]+\s*項`)
)

type tallySubject struct {
	prefix string

	what string

	// ownerSentence renders the header sentence the owner must carry, or nil
	// when the owner states no count of its own.
	ownerSentence func(ticked, open, retracted int) string

	// prose recognises this subject's count in a satellite; zero value means
	// tallyInProse.
	prose *regexp.Regexp

	satellites []string

	// nearby must appear within a few lines of a prose count for it to be
	// read as this subject's tally, not an unrelated count nearby.
	nearby []string
}

func tallySubjects() []tallySubject {
	return []tallySubject{{
		prefix: "GEN-",
		what:   "M5",
		ownerSentence: func(ticked, open, _ int) string {
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
	}, {

		prefix: "PORT-",
		what:   "M6",
		ownerSentence: func(ticked, open, retracted int) string {
			return fmt.Sprintf("完成 %d 項、撤回 %d 項、剩 %d 項", ticked, retracted, open)
		},
		prose:      portTallyInProse,
		satellites: []string{"docs/plans/01-goals-and-plan.md", "docs/plans/mvp/m6/README.md", "docs/plans/mvp/README.md", "AGENTS.md"},
		nearby:     []string{"PORT-", "§20", "M6"},
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
	withdrawn := regexp.MustCompile(`(?m)^- \[~\] ~~` + s.prefix)
	ticked := len(checked.FindAllString(owner, -1))
	open := len(unchecked.FindAllString(owner, -1))
	retracted := len(withdrawn.FindAllString(owner, -1))
	prose := s.prose
	if prose == nil {
		prose = tallyInProse
	}
	if ticked+open == 0 {
		return []string{fmt.Sprintf(
			"milestone-tally: %s has no `- [x] %s` / `- [ ] %s` items; this check has lost %s's subject",
			tallyOwner, s.prefix, s.prefix, s.what)}
	}

	var problems []string

	if s.ownerSentence != nil {
		want := s.ownerSentence(ticked, open, retracted)
		if !strings.Contains(owner, want) {
			problems = append(problems, fmt.Sprintf(
				"milestone-tally: %s has %d ticked and %d open %s items, so its section header must say %q and does not",
				tallyOwner, ticked, open, s.prefix, want))
		}
	}

	for _, rel := range s.satellites {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		text := string(body)
		for _, loc := range prose.FindAllStringIndex(text, -1) {
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
