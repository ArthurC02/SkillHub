package skillpkg

import (
	"regexp"
	"sort"
	"strings"
)

// The placeholder census: which fill-me-in shapes appear in a SKILL.md body.
//
// A measurement tool for GEN-009, not a validator: none of this blocks
// anything, and the count is a proxy — it separates "there are instructions
// here" from "there is a shape here" and nothing beyond that
// (m5/report-generate-spike.md §3.2).
//
// The first version lived in the spike harness and measured 0 true positives
// against 2 false ones over the A round, which the report recorded as
// systematic: a Skill ABOUT scanning TODOs tripped the TODO rule mid-sentence
// (OUT-3), a filename template in explanatory prose tripped the angle rule
// (DOC-4), the angle regex matched every lowercase HTML tag, and the ellipsis
// rule matched standard elision lines inside code blocks. Each of those has a
// shape the real thing does not share, and this version keys on the
// difference:
//
//   - TODO/FIXME count only as LINE-LEADING markers (`TODO: …`, `- FIXME …`).
//     Prose that mentions the word mid-sentence is a topic, not a slot.
//   - Fenced code blocks are stripped before scanning. Code legitimately
//     contains `...`, angle-bracketed type parameters and TODO comments that
//     are the generated example's content, not the package's blanks.
//   - An angle slot must contain a SPACE (`<your name here>`), which neither
//     an HTML tag nor a `<original-name>` filename template does.
//   - A heading with nothing under it before the next heading or EOF is a
//     shape of its own (`empty-section`) — the B round's 38-character shell
//     was exactly headings with no bodies, and no word-list catches that.
//
// Exported from skillpkg (rather than kept in the test harness) so the unit
// test pinning the recorded false positives runs on every CI pass instead of
// only when someone sets GENERATED_CORPUS.
var placeholderRules = map[string]*regexp.Regexp{
	// (?m) line-leading, optionally behind list/heading/quote markers.
	"TODO":  regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?TODO\b`),
	"FIXME": regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?FIXME\b`),
	// A slot, not a tag: requires an interior space.
	"<angle>":   regexp.MustCompile(`<[a-z_-]+(?: [a-z_-]+)+>`),
	"[bracket]": regexp.MustCompile(`\[(insert|your|описание|填|placeholder)[^\]]*\]`),
	"ellipsis":  regexp.MustCompile(`(?m)^\s*(\.\.\.|…)\s*$`),
	"xxx":       regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?xxx+\b`),
}

var fencedBlock = regexp.MustCompile("(?ms)^```.*?^```\\s*$")
var headingLine = regexp.MustCompile(`(?m)^(#{1,6})\s+\S`)

// PlaceholderShapes reports which placeholder shapes a SKILL.md body carries,
// sorted. body is the Markdown after the frontmatter.
func PlaceholderShapes(body string) []string {
	prose := fencedBlock.ReplaceAllString(body, "")
	var found []string
	for label, re := range placeholderRules {
		if re.MatchString(prose) {
			found = append(found, label)
		}
	}
	if hasEmptySection(prose) {
		found = append(found, "empty-section")
	}
	sort.Strings(found)
	return found
}

// hasEmptySection reports a heading with no content before the next heading of
// the same or shallower level, or the end of the document.
//
// Level matters: a title directly followed by its first subsection (# then ##)
// is ordinary structure, not a blank — the parent's content IS its children.
// What the B round's shell looked like was siblings with nothing between them.
func hasEmptySection(prose string) bool {
	locs := headingLine.FindAllStringSubmatchIndex(prose, -1)
	level := func(i int) int { return locs[i][3] - locs[i][2] } // length of the # run
	for i, loc := range locs {
		end := len(prose)
		deeperChild := false
		if i+1 < len(locs) {
			if level(i+1) > level(i) {
				deeperChild = true
			}
			end = locs[i+1][0]
		}
		if deeperChild {
			continue
		}
		section := prose[loc[1]:end]
		// Drop the rest of the heading line itself.
		if k := strings.IndexByte(section, '\n'); k >= 0 {
			section = section[k+1:]
		} else {
			section = ""
		}
		if strings.TrimSpace(section) == "" {
			return true
		}
	}
	return false
}
