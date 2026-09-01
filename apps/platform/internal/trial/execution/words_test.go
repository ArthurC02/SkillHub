package run

// The words this package puts on a screen, and the two ways they go wrong.
//
// 04 丙-115: the platform's own sentences were English on a page that declares
// `lang="zh-Hant"`, and `failure_class` was served as a bare enum that four
// screens interpolated into Chinese sentences 「失敗類別 capability_mismatch」.
// Nothing in this repository read for language, so every suite stayed green.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// The label table and the CHECK constraint are one vocabulary in two files, and
// nothing but this test holds them together.
//
// A missing entry does not fail loudly: failureClassWord falls back to showing
// the raw value, which is deliberate (a word the reader must look up beats a
// blank) and therefore invisible — exactly how `cleanup_status` shipped a blank
// row for the one state it existed to report (04 丙-28/29 ②). The constraint is
// read from the migration rather than restated here, because a copy of a list is
// a second list.
func TestFailureClassWordsCoverExactlyTheVocabularyTheDatabaseAllows(t *testing.T) {
	const migration = "../../../../../db/migrations/0018_run_scheduling.sql"
	src, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}
	i := strings.Index(string(src), "CHECK (failure_class IS NULL OR failure_class IN (")
	if i < 0 {
		t.Fatalf("%s no longer declares the failure_class CHECK this test reads", migration)
	}
	j := strings.Index(string(src)[i:], "));")
	if j < 0 {
		t.Fatalf("%s: the failure_class CHECK is not closed where expected", migration)
	}
	allowed := regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(string(src)[i:i+j], -1)
	if len(allowed) == 0 {
		t.Fatalf("%s: read no values out of the failure_class CHECK", migration)
	}

	want := make([]string, 0, len(allowed))
	for _, m := range allowed {
		want = append(want, m[1])
	}
	got := make([]string, 0, len(failureClassWords))
	for k := range failureClassWords {
		got = append(got, k)
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Errorf("failureClassWords and the database disagree about what a failure class can be.\n"+
			"  database (%s): %s\n  failureClassWords:      %s", migration,
			strings.Join(want, ", "), strings.Join(got, ", "))
	}

	for _, v := range want {
		l := failureClassWord(v)
		if l == nil {
			t.Fatalf("%s: no label at all", v)
		}
		if !hasHan(l.Label) {
			t.Errorf("%s: label %q is not in the interface language", v, l.Label)
		}
		// The note is where each class stops being read wrong — workload_error
		// reads as a platform fault, capability_mismatch reads as a crash — and
		// 設計 §2.4 makes it visible text rather than a tooltip, so an empty one
		// renders as an empty paragraph.
		if !hasHan(l.Note) {
			t.Errorf("%s: note %q is not in the interface language", v, l.Note)
		}
	}

	// An empty class is absent, not a labelled blank: `failure_class` is omitted
	// for a run that did not fail, and `{value:"",label:"",note:""}` on every
	// successful run would render as an empty note paragraph.
	if failureClassWord("") != nil {
		t.Error("an empty failure class produced a label instead of being omitted")
	}
	// And an unknown one still says something rather than vanishing.
	if l := failureClassWord("something_new"); l == nil || l.Label != "something_new" {
		t.Errorf("an unrecognised class must fall back to showing its raw value, got %+v", l)
	}
}

// The happy path's own sentences. These are what every successful run shows in
// 進度 on /runs/{id} — six lines that were English until 2026-09-01, on the
// screen the whole product exists to produce.
//
// Only the platform's own vocabulary is asserted. The same field also carries
// sentences relayed verbatim from the provider and the text of Go errors, and
// neither is ours to write; public.yaml's Run.status_reason says so, and the web
// app relays them unchanged.
func TestTheHappyPathSpeaksTheInterfaceLanguage(t *testing.T) {
	for _, to := range []gen.RunStatus{
		gen.RunStatusPreparing,
		gen.RunStatusRunning,
		gen.RunStatusEvaluating,
		gen.RunStatusSucceeded,
	} {
		if r := successReason(to); !hasHan(r) {
			t.Errorf("the reason recorded for %s is not in the interface language: %q", to, r)
		}
	}
	// The terminal one carries ADR-025's separation, and that is the sentence a
	// reader most needs to understand: a workload that finished is not a task
	// that was achieved.
	final := successReason(gen.RunStatusSucceeded)
	if !strings.Contains(final, "評估") {
		t.Errorf("the terminal reason no longer points at the evaluation as the separate judgement: %q", final)
	}
}
