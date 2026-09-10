package skillpkg

import (
	"regexp"
	"sort"
	"strings"
)

var placeholderRules = map[string]*regexp.Regexp{

	"TODO":  regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?TODO\b`),
	"FIXME": regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?FIXME\b`),

	"<angle>":   regexp.MustCompile(`<[a-z_-]+(?: [a-z_-]+)+>`),
	"[bracket]": regexp.MustCompile(`\[(insert|your|описание|填|placeholder)[^\]]*\]`),
	"ellipsis":  regexp.MustCompile(`(?m)^\s*(\.\.\.|…)\s*$`),
	"xxx":       regexp.MustCompile(`(?mi)^\s*(?:[-*>#]+\s+)?xxx+\b`),
}

const codeMarker = "\u0000code\u0000"

// codeShapes match fenced (backtick or tilde) blocks and indented blocks, so
// stripCode can mask them before the placeholder rules scan prose.
var codeShapes = []*regexp.Regexp{

	regexp.MustCompile("(?ms)^[ \t]*(?:```|~~~).*?(?:^[ \t]*(?:```|~~~)[ \t]*$|\\z)"),

	regexp.MustCompile(`(?m)^(?:(?:[ ]{4}|\t).*\n?)+`),
}

var headingLine = regexp.MustCompile(`(?m)^(#{1,6})\s+\S`)

func stripCode(body string) string {
	out := body
	for _, re := range codeShapes {
		out = re.ReplaceAllString(out, codeMarker+"\n")
	}
	return out
}

func PlaceholderShapes(body string) []string {
	prose := stripCode(body)
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

// hasEmptySection walks headings pairwise: a heading immediately followed by
// a deeper one owns that content, so only headings with no deeper child are
// checked for a blank body before the next heading of the same level or EOF.
func hasEmptySection(prose string) bool {
	locs := headingLine.FindAllStringSubmatchIndex(prose, -1)
	level := func(i int) int { return locs[i][3] - locs[i][2] }
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
