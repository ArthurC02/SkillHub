package catalog

import (
	"reflect"
	"testing"
)

// The tokenizer mirrors tools/goldenset/evaluate.py's tokenize; the F1 numbers
// in creation-measure/search-f1 were produced with that function, so a drift
// here would make them numbers about a leg nobody runs.
func TestLexicalTokensMirrorTheGoldenSetTokenizer(t *testing.T) {
	got := LexicalTokens("name: pii-flag 我要把會議記錄 整理 csv-to-json v2 一")
	want := []string{"name", "pii-flag", "csv-to-json", "v2", "我要", "要把", "把會", "會議", "議記", "記錄", "整理", "一"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens\n got %v\nwant %v", got, want)
	}
}

// Every token AND-ed is the coverage rule; a repeated token appears once; an
// empty query renders empty so the caller skips the leg.
func TestLexicalQueryRendersTheCoverageRule(t *testing.T) {
	if got := lexicalQuery("Excel 去重 excel", "&"); got != "excel & 去重" {
		t.Fatalf("got %q", got)
	}
	if got := lexicalQuery("去重複", "|"); got != "去重 | 重複" {
		t.Fatalf("got %q", got)
	}
	if got := lexicalQuery("   ", "&"); got != "" {
		t.Fatalf("empty query must render empty, got %q", got)
	}
}

// Two rewrites agreeing on a document outrank either's lone first hit.
func TestFuseRankedRewardsAgreementAcrossRewrites(t *testing.T) {
	got := FuseRanked([][]string{{"a", "b", "c"}, {"b", "d"}, {"b", "a"}})
	if !reflect.DeepEqual(got, []string{"b", "a", "d", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := FuseRanked(nil); len(got) != 0 {
		t.Fatalf("nothing in, nothing out: %v", got)
	}
}
