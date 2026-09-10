package run

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

		if !hasHan(l.Note) {
			t.Errorf("%s: note %q is not in the interface language", v, l.Note)
		}
	}

	if failureClassWord("") != nil {
		t.Error("an empty failure class produced a label instead of being omitted")
	}

	if l := failureClassWord("something_new"); l == nil || l.Label != "something_new" {
		t.Errorf("an unrecognised class must fall back to showing its raw value, got %+v", l)
	}
}

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

	final := successReason(gen.RunStatusSucceeded)
	if !strings.Contains(final, "評估") {
		t.Errorf("the terminal reason no longer points at the evaluation as the separate judgement: %q", final)
	}
}
