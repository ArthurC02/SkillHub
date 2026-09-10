package catalog

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	lexicalWord = regexp.MustCompile(`[a-z0-9][a-z0-9+.#_-]*`)
	lexicalCJK  = regexp.MustCompile(`[\x{4e00}-\x{9fff}]+`)
)

func LexicalTokens(text string) []string {
	low := strings.ToLower(text)
	tokens := lexicalWord.FindAllString(low, -1)
	for _, run := range lexicalCJK.FindAllString(low, -1) {
		r := []rune(run)
		if len(r) == 1 {
			tokens = append(tokens, run)
			continue
		}
		for i := 0; i+1 < len(r); i++ {
			tokens = append(tokens, string(r[i:i+2]))
		}
	}
	return tokens
}

func LexicalIndexText(parts ...string) string {
	return strings.Join(LexicalTokens(strings.Join(parts, "\n")), " ")
}

func lexicalQuery(query, op string) string {
	seen := map[string]bool{}
	var out []string
	for _, t := range LexicalTokens(query) {
		if t == "" || seen[t] || strings.IndexFunc(t, func(r rune) bool {
			return r == '&' || r == '|' || r == '!' || r == '(' || r == ')' || r == ':' || r == '\'' || unicode.IsSpace(r)
		}) >= 0 {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return strings.Join(out, " "+op+" ")
}

func FuseRanked(rankings [][]string) []string {
	score := map[string]float64{}
	var order []string
	for _, ranking := range rankings {
		for rank, id := range ranking {
			if _, seen := score[id]; !seen {
				order = append(order, id)
			}
			score[id] += 1 / float64(60+rank+1)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return score[order[i]] > score[order[j]] })
	return order
}
