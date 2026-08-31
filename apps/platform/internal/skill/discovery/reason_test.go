package catalog

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

// DISC-002/ADR-013: the `model` label means the model wrote that sentence. A
// batch that answers for some candidates and not others must not relabel the
// platform's own template sentences — the LLM service used to pad its answer
// with filler that arrived here labelled as model output
// (import-report.md §6.1 bug 3).
func TestApplyMatchReasonsLabelsEachCandidateSeparately(t *testing.T) {
	hits := []searchResult{
		{SkillID: "s1", Name: "pdf-extractor", Summary: "Extracts tables from PDF invoices"},
		{SkillID: "s2", Name: "csv-cleaner", Summary: "Normalises tabular files"},
	}
	applyMatchReasons(hits, "extract tables from an invoice pdf", []llmclient.MatchReason{
		{SkillID: "s1", Reason: "It reads invoice PDFs and returns the tables."},
		{SkillID: "s2", Reason: ""}, // covered by the batch, but with nothing in it
	})

	if hits[0].MatchReasonSource != reasonSourceModel {
		t.Fatalf("model-written reason must be labelled model, got %q", hits[0].MatchReasonSource)
	}
	if hits[1].MatchReasonSource != reasonSourceTemplate {
		t.Fatalf("empty model reason must fall back to template, got %q", hits[1].MatchReasonSource)
	}
	if hits[1].MatchReason != templateMatchReason(hits[1].Name, hits[1].Summary, "extract tables from an invoice pdf") {
		t.Fatalf("fallback must be the platform template, got %q", hits[1].MatchReason)
	}
}

// A failed batch is the whole-page case of the same rule.
func TestApplyMatchReasonsWithNoModelAnswer(t *testing.T) {
	hits := []searchResult{{SkillID: "s1", Name: "n", Summary: "s"}}
	applyMatchReasons(hits, "some task", nil)
	if hits[0].MatchReasonSource != reasonSourceTemplate || hits[0].MatchReason == "" {
		t.Fatalf("want a labelled template reason, got %+v", hits[0])
	}
}

// import-report.md §6.1 bug 2: a not-yet-enriched document has no embedding, so
// it is invisible to the ranking leg. That is index coverage, reported on its
// own field, not folded into the embed-call outage flag.
func TestAnyUnranked(t *testing.T) {
	if anyUnranked([]searchResult{{SkillID: "a"}, {SkillID: "b"}}) {
		t.Fatal("a fully ranked page is not partially indexed")
	}
	if !anyUnranked([]searchResult{{SkillID: "a"}, {SkillID: "b", unranked: true}}) {
		t.Fatal("one pending document makes the page partially indexed")
	}
}

// DISC-002: the template reason is the fallback that must never lie. Either it
// names the words the query and the document actually share, or it says the
// match came from meaning rather than wording (ADR-013 定案調整 4).
func TestTemplateMatchReason(t *testing.T) {
	tests := []struct {
		name     string
		skill    string
		summary  string
		query    string
		wantTerm string // "" = expect the no-overlap sentence
	}{
		{
			name:     "latin overlap is quoted back",
			skill:    "pdf-extractor",
			summary:  "Extracts tables from PDF invoices",
			query:    "extract tables from an invoice pdf",
			wantTerm: "tables",
		},
		{
			name:    "no overlap admits semantic match",
			skill:   "csv-cleaner",
			summary: "Normalises delimiter and encoding of tabular files",
			query:   "把發票裡的數字抓出來",
		},
		{
			// The case ADR-013 定案調整 1 measured: a Chinese query against an
			// English document. There is nothing lexical to report, and the
			// reason must not pretend otherwise.
			name:    "cross-language hit has no shared keywords",
			skill:   "invoice-parser",
			summary: "Reads scanned invoices and returns structured line items",
			query:   "幫我把掃描的發票轉成表格",
		},
		{
			name:     "cjk overlap is reported as the phrase, not as bigrams",
			skill:    "資料分析助手",
			summary:  "產生報表",
			query:    "我要做資料分析",
			wantTerm: "資料分析",
		},
		{
			name:    "stopword-only overlap is not a reason",
			skill:   "widget",
			summary: "You can use this for that",
			query:   "how can you use the thing",
		},
	}

	const noOverlap = "沒有共同的關鍵字"
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := templateMatchReason(tc.skill, tc.summary, tc.query)
			if tc.wantTerm == "" {
				if !strings.Contains(got, noOverlap) {
					t.Fatalf("want the no-overlap sentence, got %q", got)
				}
				return
			}
			if strings.Contains(got, noOverlap) {
				t.Fatalf("want overlap on %q, got the no-overlap sentence", tc.wantTerm)
			}
			if !strings.Contains(got, tc.wantTerm) {
				t.Fatalf("want reason naming %q, got %q", tc.wantTerm, got)
			}
		})
	}
}

// The reason has to stay one readable line even when the query repeats the
// document back at it.
func TestOverlapTermsIsCappedAndDeduplicated(t *testing.T) {
	doc := "alpha bravo charlie delta echo foxtrot golf hotel"
	query := "alpha alpha bravo charlie delta echo foxtrot golf hotel"
	got := overlapTerms(query, doc)
	if len(got) > 5 {
		t.Fatalf("want at most 5 terms, got %d: %v", len(got), got)
	}
	seen := map[string]bool{}
	for _, term := range got {
		if seen[term] {
			t.Fatalf("duplicate term %q in %v", term, got)
		}
		seen[term] = true
	}
}

func TestTokenize(t *testing.T) {
	// Latin runs shorter than three characters are dropped: "of"/"a" matching
	// is noise, not evidence.
	got := tokenize("Read a PDF of Tables")
	want := []string{"read", "pdf", "tables"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("latin: got %v, want %v", got, want)
	}

	// CJK runs become bigrams so a spaceless query still has comparable units.
	got = tokenize("資料分析")
	want = []string{"資料", "料分", "分析"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cjk: got %v, want %v", got, want)
	}

	// Mixed scripts split at the boundary rather than fusing into one token.
	got = tokenize("匯出csv檔")
	if len(got) == 0 || got[0] != "匯出" {
		t.Fatalf("mixed: got %v, want a leading 匯出 token", got)
	}
	if !contains(got, "csv") {
		t.Fatalf("mixed: got %v, want the latin run kept as csv", got)
	}
}

// DISC-001: a blank or unusable query must not turn into a search.
//
// The negative cases are the point. Every one of them is two runes long, so a
// rule that only counts runes admits all of them - and each admission costs a
// gateway embedding and a funnel event that says a search happened.
func TestIsComprehensible(t *testing.T) {
	unsearchable := []string{
		"", " ", "a", // blank, blank-ish, one character
		"  ", // two spaces
		"!!", // two ASCII punctuation marks
		"、、", // two CJK punctuation marks, which are not CJK words
		"—",  // an em dash
		"你",  // a single ideograph is one unit, not two
		"1",  // a single digit
	}
	for _, q := range unsearchable {
		if isComprehensible(q) {
			t.Errorf("%q should not be searchable", q)
		}
	}

	// Scripts this catalog can be searched in. The three non-Latin, non-CJK ones
	// were rejected by the hand-written character range and only got through on
	// the rune-count fallback that made the range meaningless.
	searchable := []string{
		"pdf tables", // Latin
		"轉表格",        // CJK ideographs
		"データ",        // Katakana
		"데이터",        // Hangul
		"данные",     // Cyrillic
		"a1",         // one letter and one digit
		// Language and product names, which a Skill catalog gets asked about
		// and which the first version of this fix refused outright by demanding
		// two letters.
		"C#", "C++", "F#", "R2",
	}
	for _, q := range searchable {
		if !isComprehensible(q) {
			t.Errorf("%q should be searchable", q)
		}
	}
}

// latinProse is a run of Latin letters long enough to be a word of English
// rather than a term quoted back from the user's own query ("pdf", "csv") or a
// product name. Three is the tokenizer's own floor for "this is a word".
var latinProse = regexp.MustCompile(`[A-Za-z]{3,}`)

// The interface is Traditional Chinese and the front end renders these strings
// verbatim — it keeps no translation table (WorkspaceRuns.tsx:65-83), so the
// server's words are the user's words.
//
// This test exists because nothing else held the line. Every assertion on this
// copy — here and in disc_integration_test.go — checked only that the string
// was non-empty, so three English sentences shipped on a Chinese search page
// and no test went red for it. Non-empty is not a language.
func TestUserFacingSearchCopySpeaksTheInterfaceLanguage(t *testing.T) {
	cases := []struct {
		what string
		got  string
	}{
		{"noResultsSuggestion", noResultsSuggestion},
		// The no-overlap reason: no query terms can leak into it by definition.
		{"templateMatchReason (no overlap)", templateMatchReason("csv-cleaner", "Normalises tabular files", "把發票裡的數字抓出來")},
		// The overlap reason, with a CJK query so the quoted term is not Latin
		// either and the whole sentence must be Chinese.
		{"templateMatchReason (overlap)", templateMatchReason("資料分析助手", "產生報表", "我要做資料分析")},
	}
	for _, tc := range cases {
		if !strings.ContainsFunc(tc.got, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Errorf("%s has no Han characters at all, so it is not the interface language: %q", tc.what, tc.got)
		}
		if w := latinProse.FindString(tc.got); w != "" {
			t.Errorf("%s reads as English prose (%q): %q", tc.what, w, tc.got)
		}
	}

	// GEN-004 anchors the generation entry point on the page's own
	// "沒有夠接近的 Skill。" (Home.tsx). The server's suggestion sits directly
	// under it, so repeating the sentence would give the anchor two candidates
	// and the user the same line twice.
	if strings.Contains(noResultsSuggestion, "沒有夠接近的 Skill") {
		t.Errorf("the suggestion repeats the page's own no-results line: %q", noResultsSuggestion)
	}

	// DISC-001 條 3 wants all three, not the two the English copy asked for.
	for _, want := range []string{"任務", "輸入", "輸出"} {
		if !strings.Contains(noResultsSuggestion, want) {
			t.Errorf("DISC-001 asks the user for 任務/輸入/輸出; the suggestion never says %q: %q", want, noResultsSuggestion)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// 04 丙-112. `q` arrives as whatever bytes percent-decoding produced, and Go
// strings do not validate them, so a query can reach this gate holding
// something that is not text at all.
//
// The three cases below are the ones that got through: ranging over a malformed
// string yields utf8.RuneError for the bad bytes only, so a single letter beside
// them satisfied "two runes, one of them a letter". PostgreSQL then answered
// SQLSTATE 22021 and the anonymous endpoint answered 500 -- and in clean mode
// that error took the whole deployment with it.
//
// The valid strings are here so the fix cannot be "reject anything unusual":
// every script the test above admits still has to pass.
func TestIsComprehensibleRefusesBytesThatAreNotText(t *testing.T) {
	notText := map[string]string{
		"a lone Big5 lead byte beside a letter": "\xa7A",
		"a truncated UTF-8 sequence":            "pdf \xe4\xb8",
		"a raw byte in the middle of a word":    "sp\xffreadsheet",
	}
	for name, q := range notText {
		if isComprehensible(q) {
			t.Errorf("%s (%q) was accepted as a query; it reaches websearch_to_tsquery as invalid UTF-8", name, q)
		}
	}

	// The regression this must not cause.
	for _, q := range []string{"pdf tables", "轉表格", "データ", "데이터", "данные", "C++", "🎉 party"} {
		if !isComprehensible(q) && utf8.ValidString(q) {
			t.Errorf("%q is valid text and stopped being searchable", q)
		}
	}
}
