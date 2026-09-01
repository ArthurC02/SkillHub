package eval

// verdictOf had no test at all, and the way it hid is worth stating: RunVerdicts
// sits at 64.7% coverage, so the read looked exercised. Every test that reaches
// it passes run ids with no evaluation row, takes the `notEvaluated` blank path
// and returns before the loop — so the six sentences a user actually reads about
// whether their task succeeded were produced by nothing.
//
// That is the same shape as 04 丙-28/29 ②, where `cleanup_status` shipped a blank
// row for the one state it existed to report and every suite stayed green.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The two distinctions ADR-025 and 02:EVAL-001 actually require, and the only
// two that cost a user something when they collapse.
//
// (1) A pending or failed *evaluation* is not a verdict about the task. 02:RUN-002
// keeps 「執行狀態」 and 「任務判定」 on two axes precisely because 「執行完成」 beside a
// bad verdict is a real and common combination; folding an evaluation failure
// into 「未符合」 tells someone their skill failed when nobody judged it.
//
// (2) `undetermined` is a verdict the judge REACHED on insufficient evidence, not
// a statement that no judge ran. 02:EVAL-001 makes the difference load-bearing:
// 「`undetermined` 與判錯分開計數」 in EVAL-013's regression, and 「不得只提供無法解釋
// 的分數」 here. 未評估 is the other one, and it is a different value.
func TestAnEvaluationThatDidNotFinishIsNotAVerdictAboutTheTask(t *testing.T) {
	for _, status := range []string{"pending", "failed"} {
		// `overall` is NOT NULL and carries `undetermined` from creation until the
		// judge finishes, so this is the combination that actually occurs.
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

	// The other half of the same distinction: a judge that ran and could not
	// conclude is not a judge that never ran.
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

// Every value the database can store gets its own sentence, read from the CHECK
// rather than restated here — a copy of a list is a second list.
//
// The failure this catches is silent by construction: verdictOf's `overall`
// switch ends in `default`, so a seventh verdict added to the column would be
// rendered as 「無法判斷」 with no test going red and no screen looking broken.
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

	// And none of them collides with the no-evaluation value, which is served
	// from the same field by the same read.
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
