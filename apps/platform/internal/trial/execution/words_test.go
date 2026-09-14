package run

import (
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

func TestFailureClassWordsCoverExactlyTheClosedList(t *testing.T) {
	classes := AllFailureClasses()
	if len(classes) == 0 {
		t.Fatal("the closed list of failure classes is empty")
	}
	if len(failureClassWords) != len(classes) {
		t.Errorf("failureClassWords describes %d classes, the closed list has %d", len(failureClassWords), len(classes))
	}

	for _, class := range classes {
		if _, ok := failureClassWords[class]; !ok {
			t.Errorf("%s: failureClassWords has no words for it", class)
			continue
		}
		l := failureClassWord(string(class))
		if !hasHan(l.Label) {
			t.Errorf("%s: label %q is not in the interface language", class, l.Label)
		}
		if !hasHan(l.Note) {
			t.Errorf("%s: note %q is not in the interface language", class, l.Note)
		}
	}

	if failureClassWord("") != nil {
		t.Error("an empty failure class produced a label instead of being omitted")
	}

	if l := failureClassWord("something_new"); l == nil || l.Label != "something_new" {
		t.Errorf("an unrecognised class must fall back to showing its raw value, got %+v", l)
	}
}

func TestProviderSideFailuresAreFiledUnderProvision(t *testing.T) {
	provision := map[FailureClass]bool{failureProvider: true, failureNoProvider: true}
	for _, class := range AllFailureClasses() {
		want := "execution"
		if provision[class] {
			want = "provision"
		}
		if got := class.category(); got != want {
			t.Errorf("%s: category = %q, want %q", class, got, want)
		}
	}
}

func TestOnlyAProviderErrorIsRetryable(t *testing.T) {
	for _, class := range AllFailureClasses() {
		if got, want := class.retryable(), class == failureProvider; got != want {
			t.Errorf("%s: retryable = %v, want %v", class, got, want)
		}
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
