package eval

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func TestAnEvaluationThatDidNotFinishIsNotAVerdictAboutTheTask(t *testing.T) {
	for _, status := range []string{"pending", "failed"} {

		got := verdictOf(status, "undetermined")
		if got.Value == "not_met" || got.Value == "undetermined" {
			t.Errorf("verdictOf(%q, \"undetermined\").Value = %q; an evaluation that "+
				"has not produced a verdict must not read as one the judge reached",
				status, got.Value)
		}
		if !hasHan(got.Label) || !hasHan(got.Note) {
			t.Errorf("verdictOf(%q, …) = %+v; the interface declares lang=zh-Hant", status, got)
		}
	}

	failed := verdictOf("failed", "undetermined")
	if !strings.Contains(failed.Note, "不代表任務失敗") {
		t.Errorf("verdictOf(\"failed\", …).Note = %q; it must say outright that the "+
			"evaluation failing is not the task failing", failed.Note)
	}

	undetermined := verdictOf("completed", "undetermined")
	if undetermined.Value != "undetermined" {
		t.Fatalf("verdictOf(\"completed\", \"undetermined\").Value = %q, want undetermined",
			undetermined.Value)
	}
	if undetermined.Value == notEvaluated.Value || undetermined.Label == notEvaluated.Label {
		t.Errorf("a judged-but-inconclusive run reads the same as an unjudged one (%+v vs %+v)",
			undetermined, notEvaluated)
	}
	if !strings.Contains(undetermined.Note, "不是判定沒跑") {
		t.Errorf("verdictOf(\"completed\", \"undetermined\").Note = %q; 02:EVAL-001 counts "+
			"undetermined separately from a wrong verdict, so the copy has to say which it is",
			undetermined.Note)
	}
}

func TestEveryVerdictTheDatabaseAllowsHasItsOwnSentence(t *testing.T) {
	const migration = "../../../../../db/migrations/0004_test_lab_and_runs.sql"
	src, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}
	i := strings.Index(string(src), "CHECK (overall IN (")
	if i < 0 {
		t.Fatalf("%s no longer declares the overall CHECK this test reads", migration)
	}
	j := strings.Index(string(src)[i:], "))")
	if j < 0 {
		t.Fatalf("%s: the overall CHECK is not closed where expected", migration)
	}
	found := regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(string(src)[i:i+j], -1)
	if len(found) < 4 {
		t.Fatalf("%s: read %d values out of the overall CHECK, want the four it declares",
			migration, len(found))
	}

	seen := map[string]string{}
	for _, m := range found {
		overall := m[1]
		got := verdictOf("completed", overall)
		if got.Value != overall {
			t.Errorf("verdictOf(\"completed\", %q).Value = %q; a completed evaluation must "+
				"carry the verdict the database stored, not a fallback", overall, got.Value)
		}
		if !hasHan(got.Label) || !hasHan(got.Note) {
			t.Errorf("verdictOf(\"completed\", %q) = %+v; label and note must be readable "+
				"in the interface's language", overall, got)
		}
		if prev, dup := seen[got.Label]; dup {
			t.Errorf("%q and %q render the same label %q; two verdicts a user must "+
				"distinguish cannot share their only visible difference", prev, overall, got.Label)
		}
		seen[got.Label] = overall
	}

	if _, clash := seen[notEvaluated.Label]; clash {
		t.Errorf("a real verdict renders as %q, the same as a run with no evaluation at all",
			notEvaluated.Label)
	}

	labels := make([]string, 0, len(seen))
	for l := range seen {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	if len(labels) != len(found) {
		t.Errorf("the database allows %d verdicts but they render as %d distinct labels: %v",
			len(found), len(labels), labels)
	}
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
