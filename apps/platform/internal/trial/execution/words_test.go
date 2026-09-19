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
		if r := successReason(to); !hasHan(string(r)) {
			t.Errorf("the reason recorded for %s is not in the interface language: %q", to, r)
		}
	}

	final := successReason(gen.RunStatusSucceeded)
	if !strings.Contains(string(final), "評估") {
		t.Errorf("the terminal reason no longer points at the evaluation as the separate judgement: %q", final)
	}
}

func TestEveryFailurePathSpeaksTheInterfaceLanguage(t *testing.T) {
	for _, class := range AllFailureClasses() {
		if r := platformWordingFor(class); !hasHan(string(r)) {
			t.Errorf("%s: the reason a run carries when nothing more specific is known is not in the "+
				"interface language: %q", class, r)
		}
	}

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"a sandbox that did not answer", ErrProviderUnavailable},
		{"a sandbox that refused", &providerError{Status: 422}},
		{"a gateway that would not mint a key", &gatewayError{Status: 500, Message: "budget exhausted"}},
		{"a deployment with no model gateway", ErrNoModelGateway},
		{"material the clean test mode will not run", ErrContentNotCurated},
		{"a request no configured sandbox can carry", ErrNoCompatibleProvider},
		{"a deployment with no sandbox at all", ErrNoProvider},
	} {
		if r := namedRefusal(tc.err); !hasHan(string(r)) {
			t.Errorf("%s: the named refusal is not in the interface language: %q", tc.name, r)
		}
	}

	silent := ProviderRun{Result: &RunResult{}}
	for _, tc := range []struct {
		name string
		run  ProviderRun
	}{
		{"a terminal state with no result at all", ProviderRun{State: ProviderStateFailed}},
		{"a cancellation the provider did not explain", withStatus(silent, "cancelled")},
		{"a timeout the provider did not explain", withStatus(silent, "timed_out")},
		{"a workload failure the provider did not explain",
			withState(withStatus(silent, "failed"), ProviderStateCompleted)},
		{"a provider failure it did not explain", withStatus(silent, "failed")},
	} {
		_, _, _, message := classifyResult(tc.run)
		if !hasHan(string(message)) {
			t.Errorf("%s: the reason the run carries is not in the interface language: %q", tc.name, message)
		}
	}
}

func withStatus(pr ProviderRun, status string) ProviderRun {
	result := *pr.Result
	result.Status = status
	pr.Result = &result
	return pr
}

func withState(pr ProviderRun, state ProviderRunState) ProviderRun {
	pr.State = state
	return pr
}
