package catalog

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// The lexical leg of the creation tool's hybrid retrieval (05 R-47,
// creation-measure/search-f1, 2026-09-06).
//
// The english tsvector cannot see a Traditional Chinese query at all — on the
// M1 golden query set replayed through the product SQL it scores F1@3 0.02 —
// and the vector leg alone, cut at CreationMaxDistance, misses what a person
// types when they know the name or one distinctive term (23/31 exact skill
// names within the cutoff, 1/25 distinctive terms). Tokenising the way the
// golden set's evaluator does (latin words plus CJK character bigrams, no
// segmenter dependency) and admitting a lexical hit only when it covers every
// query token gives F1 0.88 over golden + name + term queries against 0.59 for
// the vector leg, with the golden set's own F1 (0.77) and its twelve distractor
// rejections unchanged.

var (
	lexicalWord = regexp.MustCompile(`[a-z0-9][a-z0-9+.#_-]*`)
	lexicalCJK  = regexp.MustCompile(`[\x{4e00}-\x{9fff}]+`)
)

// LexicalTokens mirrors tools/goldenset/evaluate.py's tokenize: lowercase
// latin words (with the punctuation skill names carry), and for every CJK run
// its overlapping character bigrams (a single character stands alone).
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

// LexicalIndexText is what the bigram column stores: the tokens joined by
// spaces, so to_tsvector('simple', …) keeps each one as its own lexeme.
func LexicalIndexText(parts ...string) string {
	return strings.Join(LexicalTokens(strings.Join(parts, "\n")), " ")
}

// lexicalQuery renders the query's distinct tokens for to_tsquery('simple'),
// joined by op ("&" admits a document only when every token is present — the
// coverage rule the measurement settled on; "|" is the degraded fallback when
// no embedding is available). Tokens contain only [a-z0-9+.#_-] and CJK
// characters, none of tsquery's operators, so they need no quoting.
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

// FuseRanked merges several rankings of the same catalogue (one per query
// rewrite) by reciprocal rank (k = 60): an id ranked well by several rewrites
// rises, one that only one rewrite found stays in but lower. Order within a
// tie follows the first ranking that produced the id.
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
