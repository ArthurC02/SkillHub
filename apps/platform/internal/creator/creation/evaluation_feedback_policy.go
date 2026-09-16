package creation

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func copiedFromEvaluation(evaluationText, draftText string, theirs ...string) []string {
	if strings.TrimSpace(evaluationText) == "" {
		return nil
	}
	fromJudge := markerSegments(evaluationText)
	if len(fromJudge) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, t := range theirs {
		for _, seg := range alphanumericSegments(t) {
			known[seg] = true
		}
	}
	var copied []string
	seen := map[string]bool{}
	for token, segments := range markerTokens(draftText) {
		for _, seg := range segments {
			if fromJudge[seg] && !known[seg] && !seen[token] {
				seen[token] = true
				copied = append(copied, token)
			}
		}
	}

	sort.Strings(copied)
	return copied
}

func toolsNotRequested(prevTools, curTools string, theirs ...string) []string {
	added := addedToolTokens(prevTools, curTools)
	if len(added) == 0 {
		return nil
	}
	var out []string
	for _, tool := range added {
		if !asked(toolBaseName(tool), theirs) {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}

var negations = []string{"不要", "不用", "不需要", "不能", "別用", "別", "禁止", "勿", "無需", "沒有要",
	"don't", "do not", "dont", "no ", "not ", "never", "without", "avoid", "except"}

const negationWindow = 16

func asked(tool string, theirs []string) bool {
	needle := strings.ToLower(strings.TrimSpace(tool))
	if needle == "" {
		return false
	}
	for _, t := range theirs {
		hay := strings.ToLower(t)
		for at := 0; ; {
			i := strings.Index(hay[at:], needle)
			if i < 0 {
				break
			}
			i += at
			if !negated(hay, i) {
				return true
			}
			at = i + len(needle)
		}
	}
	return false
}

func negated(hay string, i int) bool {
	start := i
	for n := 0; start > 0 && n < negationWindow; n++ {
		_, size := utf8.DecodeLastRuneInString(hay[:start])
		start -= size
	}
	before := hay[start:i]
	for _, n := range negations {
		if strings.Contains(before, n) {
			return true
		}
	}
	return false
}

func toolsNamedIn(evaluationText string, tools []string) []string {
	if strings.TrimSpace(evaluationText) == "" {
		return nil
	}
	hay := strings.ToLower(evaluationText)
	var named []string
	for _, tool := range tools {
		if base := strings.ToLower(toolBaseName(tool)); base != "" && strings.Contains(hay, base) {
			named = append(named, tool)
		}
	}
	return named
}

func addedToolTokens(prev, cur string) []string {
	old := map[string]bool{}
	for _, t := range toolFields(prev) {
		old[strings.ToLower(t)] = true
	}
	var added []string
	seen := map[string]bool{}
	for _, t := range toolFields(cur) {
		key := strings.ToLower(t)
		if !old[key] && !seen[key] {
			seen[key] = true
			added = append(added, t)
		}
	}
	return added
}

func toolFields(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

func toolBaseName(tool string) string {
	if i := strings.IndexByte(tool, '('); i >= 0 {
		return tool[:i]
	}
	return tool
}

func markerTokens(s string) map[string][]string {
	out := map[string][]string{}
	for _, token := range tokens(s) {
		if segs := markerLike(token); len(segs) > 0 {
			out[token] = segs
		}
	}
	return out
}

func markerSegments(s string) map[string]bool {
	out := map[string]bool{}
	for _, segs := range markerTokens(s) {
		for _, seg := range segs {
			out[seg] = true
		}
	}
	return out
}

func alphanumericSegments(s string) []string {
	var out []string
	for _, token := range tokens(s) {
		for _, seg := range strings.FieldsFunc(token, isSeparator) {
			if len([]rune(seg)) >= 2 {
				out = append(out, seg)
			}
		}
	}
	return out
}

func isSeparator(r rune) bool { return r == '-' || r == '_' }

func tokens(s string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !isSeparator(r)
	}) {
		if field = strings.Trim(field, "-_"); field != "" {
			out = append(out, field)
		}
	}
	return out
}

// markerLike flags a token's ASCII segments that mix letters and digits: a
// hyphen/underscore-joined segment counts at length 4+, a bare segment only
// at length 8+ with at least two of each character class.
func markerLike(token string) []string {
	segments := strings.FieldsFunc(token, isSeparator)
	compound := len(segments) > 1
	var found []string
	for _, seg := range segments {
		letters, digits := 0, 0
		ascii := true
		for _, r := range seg {
			switch {
			case r >= '0' && r <= '9':
				digits++
			case r >= 'a' && r <= 'z':
				letters++
			default:
				ascii = false
			}
		}
		if !ascii || letters < 1 || digits < 1 || len(seg) < 4 {
			continue
		}
		if compound || (letters >= 2 && digits >= 2 && len(seg) >= 8) {
			found = append(found, seg)
		}
	}
	return found
}
