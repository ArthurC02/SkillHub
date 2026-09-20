package run

import (
	"errors"
	"fmt"
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
	provision := map[FailureClass]bool{failureProvider: true, failureNoProvider: true, failurePolicy: true}
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

func TestEveryRefusedRunSpeaksTheInterfaceLanguage(t *testing.T) {
	if len(refusedRuns) == 0 {
		t.Fatal("no refusal carries a message of its own")
	}
	for _, r := range refusedRuns {
		if !hasHan(r.message) {
			t.Errorf("%v: the sentence the caller reads is not in the interface language: %q",
				r.is, r.message)
		}
		if r.message == r.is.Error() {
			t.Errorf("%v: the sentence the caller reads is the error's own text", r.is)
		}
	}
}

func TestARefusalNamesWhichOneItWas(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{"a deployment with no model gateway", ErrNoModelGateway, 422,
			"這個部署沒有接上模型閘道，試跑沒有辦法連到模型，請聯絡管理者。"},
		{"a workspace already at its concurrent limit",
			fmt.Errorf("%w: 2 of 2 in progress", ErrRunLimitReached), 422,
			"這個 Workspace 同時進行中的試跑已經達到上限，等其中一個結束再開始。"},
		{"a skill whose source licence is under review", ErrAccessRestricted, 422,
			"這個 Skill 的來源授權還在審查中，審查期間不能試跑。"},
		{"a deployment with no sandbox at all", ErrNoProvider, 422,
			"這個部署沒有設定任何執行沙箱，試跑沒有地方可以跑，請聯絡管理者。"},
		{"a deployment whose sandboxes cannot meet the request", ErrNoCompatibleProvider, 422,
			"現有的執行沙箱都不符合這次試跑需要的執行環境，請聯絡管理者。"},
		{"a target that is not there", ErrPreflightTargetNotFound, 404,
			messagePreflightTargetNotFound},
		{"a run that is not there", ErrNotFound, 404, messageRunNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := refusalFor(tc.err)
			if !ok {
				t.Fatalf("refusalFor(%v) found no refusal; the caller would read the error's own text", tc.err)
			}
			if got.status != tc.status || got.message != tc.want {
				t.Errorf("refusalFor(%v) = %d %q, want %d %q", tc.err, got.status, got.message, tc.status, tc.want)
			}
			if strings.Contains(got.message, "in progress") {
				t.Errorf("the sentence repeats the wrapped English detail: %q", got.message)
			}
		})
	}

	if _, ok := refusalFor(errors.New("something the platform has no sentence for")); ok {
		t.Error("an unrecognised error was matched to a refusal; it must fall through to the platform's own failure")
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
