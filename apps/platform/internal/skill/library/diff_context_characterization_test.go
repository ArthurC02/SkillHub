package registry

import (
	"strings"
	"testing"
)

func TestAVersionDiffShowsThreeLinesOfContextAroundAChange(t *testing.T) {
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	after := strings.Replace(before, "five\n", "FIVE\n", 1)
	got := textDiff("SKILL.md", []byte(before), []byte(after))
	if !strings.Contains(got, "@@ -2,7 +2,7 @@\n two\n three\n four\n-five\n+FIVE\n six\n seven\n eight\n") {
		t.Fatalf("diff =\n%s\nwant one hunk carrying lines two to eight", got)
	}
}
