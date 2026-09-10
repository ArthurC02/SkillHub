package catalog

import (
	"reflect"
	"testing"
)

func TestLexicalTokensMirrorTheGoldenSetTokenizer(t *testing.T) {
	got := LexicalTokens("name: pii-flag 我要把會議記錄 整理 csv-to-json v2 一")
	want := []string{"name", "pii-flag", "csv-to-json", "v2", "我要", "要把", "把會", "會議", "議記", "記錄", "整理", "一"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens\n got %v\nwant %v", got, want)
	}
}

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

func TestFuseRankedRewardsAgreementAcrossRewrites(t *testing.T) {
	got := FuseRanked([][]string{{"a", "b", "c"}, {"b", "d"}, {"b", "a"}})
	if !reflect.DeepEqual(got, []string{"b", "a", "d", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := FuseRanked(nil); len(got) != 0 {
		t.Fatalf("nothing in, nothing out: %v", got)
	}
}
