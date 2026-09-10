package catalog

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestApplyMatchReasonsLabelsEachCandidateSeparately(t *testing.T) {
	hits := []searchResult{
		{SkillID: "s1", Name: "pdf-extractor", Summary: "Extracts tables from PDF invoices"},
		{SkillID: "s2", Name: "csv-cleaner", Summary: "Normalises tabular files"},
	}
	applyMatchReasons(hits, "extract tables from an invoice pdf", []llmclient.MatchReason{
		{SkillID: "s1", Reason: "It reads invoice PDFs and returns the tables."},
		{SkillID: "s2", Reason: ""},
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

func TestApplyMatchReasonsWithNoModelAnswer(t *testing.T) {
	hits := []searchResult{{SkillID: "s1", Name: "n", Summary: "s"}}
	applyMatchReasons(hits, "some task", nil)
	if hits[0].MatchReasonSource != reasonSourceTemplate || hits[0].MatchReason == "" {
		t.Fatalf("want a labelled template reason, got %+v", hits[0])
	}
}

func TestAnyUnranked(t *testing.T) {
	if anyUnranked([]searchResult{{SkillID: "a"}, {SkillID: "b"}}) {
		t.Fatal("a fully ranked page is not partially indexed")
	}
	if !anyUnranked([]searchResult{{SkillID: "a"}, {SkillID: "b", unranked: true}}) {
		t.Fatal("one pending document makes the page partially indexed")
	}
}

func TestTemplateMatchReason(t *testing.T) {
	tests := []struct {
		name     string
		skill    string
		summary  string
		query    string
		wantTerm string
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

	got := tokenize("Read a PDF of Tables")
	want := []string{"read", "pdf", "tables"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("latin: got %v, want %v", got, want)
	}

	got = tokenize("資料分析")
	want = []string{"資料", "料分", "分析"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cjk: got %v, want %v", got, want)
	}

	got = tokenize("匯出csv檔")
	if len(got) == 0 || got[0] != "匯出" {
		t.Fatalf("mixed: got %v, want a leading 匯出 token", got)
	}
	if !contains(got, "csv") {
		t.Fatalf("mixed: got %v, want the latin run kept as csv", got)
	}
}

func TestIsComprehensible(t *testing.T) {
	unsearchable := []string{
		"", " ", "a",
		"  ",
		"!!",
		"、、",
		"—",
		"你",
		"1",
	}
	for _, q := range unsearchable {
		if isComprehensible(q) {
			t.Errorf("%q should not be searchable", q)
		}
	}

	searchable := []string{
		"pdf tables",
		"轉表格",
		"データ",
		"데이터",
		"данные",
		"a1",

		"C#", "C++", "F#", "R2",
	}
	for _, q := range searchable {
		if !isComprehensible(q) {
			t.Errorf("%q should be searchable", q)
		}
	}
}

var latinProse = regexp.MustCompile(`[A-Za-z]{3,}`)

func TestUserFacingSearchCopySpeaksTheInterfaceLanguage(t *testing.T) {
	cases := []struct {
		what string
		got  string
	}{
		{"noResultsSuggestion", noResultsSuggestion},

		{"templateMatchReason (no overlap)", templateMatchReason("csv-cleaner", "Normalises tabular files", "把發票裡的數字抓出來")},

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

	if strings.Contains(noResultsSuggestion, "沒有夠接近的 Skill") {
		t.Errorf("the suggestion repeats the page's own no-results line: %q", noResultsSuggestion)
	}

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

	controls := map[string]string{
		"a NUL between letters": "a\x00b",
		"a NUL after a word":    "pdf\x00",
		"an escape character":   "pdf\x1b[0m",
		"a form feed":           "pdf\x0ctables",
		"a vertical tab":        "pdf\x0btables",
		"a delete character":    "pdf\x7f",
	}
	for name, q := range controls {
		if isComprehensible(q) {
			t.Errorf("%s (%q) was accepted as a query", name, q)
		}
	}

	for _, q := range []string{
		"pdf tables", "轉表格", "データ", "데이터", "данные", "C++", "🎉 party",
		"把投影片\n轉成文件", "read my\tinvoices\r\nand summarise",
	} {
		if !isComprehensible(q) && utf8.ValidString(q) {
			t.Errorf("%q is valid text and stopped being searchable", q)
		}
	}
}
