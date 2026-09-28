package eval

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestReadTargetLooksForANulByteOnlyInTheFirst8000Bytes(t *testing.T) {
	nulAt := func(i int) string {
		return strings.Repeat("a", i) + "\x00" + strings.Repeat("a", 10)
	}
	fsys := fstest.MapFS{
		"last-probed.txt":    {Data: []byte(nulAt(7999))},
		"first-unprobed.txt": {Data: []byte(nulAt(8000))},
	}

	if _, err := readTarget(fsys, "last-probed.txt"); err == nil || !strings.Contains(err.Error(), "it is not text") {
		t.Errorf("a NUL byte at offset 7999: err = %v, want the not-text refusal", err)
	}
	if got, err := readTarget(fsys, "first-unprobed.txt"); err != nil || got != nulAt(8000) {
		t.Errorf("a NUL byte at offset 8000: got %d bytes, err %v; want the file read as text", len(got), err)
	}
}

func TestTheReviewableDiffCarriesThreeLinesOfContext(t *testing.T) {
	lines := func(changed string) string {
		var b strings.Builder
		for i := 1; i <= 11; i++ {
			if i == 6 {
				b.WriteString(changed + "\n")
				continue
			}
			b.WriteString("line " + strings.Repeat("x", i) + "\n")
		}
		return b.String()
	}
	fsys := fstest.MapFS{"SKILL.md": {Data: []byte(lines("before"))}}
	sc := suggestionCtx{
		suggestion: gen.EvaluationSuggestion{TargetPath: "SKILL.md", ProposedContent: lines("after")},
		originFS:   fsys,
		latestFS:   fsys,
	}

	diff, blocked := check(sc)

	if blocked != nil {
		t.Fatalf("check blocked the suggestion: %+v", blocked)
	}
	if !strings.Contains(diff, "@@ -3,7 +3,7 @@") {
		t.Fatalf("diff does not carry three lines of context around line 6:\n%s", diff)
	}
}
