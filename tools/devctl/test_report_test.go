package main

import (
	"strings"
	"testing"
)

// events is a trimmed `go test -json` stream: two passes, three skips across
// two reasons at different files and lines, and one failure.
const events = `
{"Action":"run","Package":"p/a","Test":"TestOne"}
{"Action":"output","Package":"p/a","Test":"TestOne","Output":"=== RUN   TestOne\n"}
{"Action":"pass","Package":"p/a","Test":"TestOne","Elapsed":0}
{"Action":"output","Package":"p/a","Test":"TestTwo","Output":"    aggregate_test.go:171: SKILLHUB_TEST_DATABASE_URL not set; skipping\n"}
{"Action":"skip","Package":"p/a","Test":"TestTwo","Elapsed":0}
{"Action":"output","Package":"p/b","Test":"TestThree","Output":"    other_test.go:9: SKILLHUB_TEST_DATABASE_URL not set; skipping\n"}
{"Action":"skip","Package":"p/b","Test":"TestThree","Elapsed":0}
{"Action":"output","Package":"p/b","Test":"TestFour","Output":"    corpus_test.go:34: set SEED_CORPUS to the directory import_seed.py wrote\n"}
{"Action":"skip","Package":"p/b","Test":"TestFour","Elapsed":0}
{"Action":"pass","Package":"p/b","Test":"TestFive","Elapsed":0}
{"Action":"output","Package":"p/b","Test":"TestSix","Output":"    x_test.go:1: boom\n"}
{"Action":"fail","Package":"p/b","Test":"TestSix","Elapsed":0}
{"Action":"skip","Package":"p/c","Output":"?   p/c [no test files]\n"}
`

func TestSummarizeCountsSkipsAndGroupsThemByReason(t *testing.T) {
	s, err := summarize(strings.NewReader(strings.TrimSpace(events)), io_Discard{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Passed != 2 || s.Failed != 1 {
		t.Errorf("passed=%d failed=%d, want 2 and 1", s.Passed, s.Failed)
	}
	// The point of the whole command: a skipped test is counted, not invisible.
	if s.Skipped != 3 {
		t.Errorf("skipped=%d, want 3 (a package with no test files is not a skipped test)", s.Skipped)
	}
	// Two skips at different files and lines gave the same reason, so they must
	// land in one row — otherwise 287 identical skips print as 287 rows and the
	// report is as unreadable as the silence it replaces.
	const dbReason = "SKILLHUB_TEST_DATABASE_URL not set; skipping"
	if got := s.Reasons[dbReason]; got != 2 {
		t.Errorf("reason %q counted %d times, want 2 (file:line must not split the group)", dbReason, got)
	}
	if len(s.Reasons) != 2 {
		t.Errorf("got %d distinct reasons, want 2: %v", len(s.Reasons), s.Reasons)
	}
}

func TestSummarizeEchoesOutputSoFailuresStayVisible(t *testing.T) {
	var raw strings.Builder
	if _, err := summarize(strings.NewReader(strings.TrimSpace(events)), &raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.String(), "x_test.go:1: boom") {
		t.Error("the failure message was swallowed; the report must add to go test's output, not replace it")
	}
}

func TestReportNamesTheSkipCountAndEveryReason(t *testing.T) {
	s, err := summarize(strings.NewReader(strings.TrimSpace(events)), io_Discard{})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s.write(&out, "platform")
	got := out.String()
	for _, want := range []string{
		"2 passed, 3 skipped, 1 failed",
		"3 test(s) did not run",
		"SKILLHUB_TEST_DATABASE_URL not set; skipping",
		"set SEED_CORPUS to the directory import_seed.py wrote",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q; got:\n%s", want, got)
		}
	}
}

func TestReportSaysNothingExtraWhenNothingSkipped(t *testing.T) {
	s := testSummary{Passed: 5, Reasons: map[string]int{}}
	var out strings.Builder
	s.write(&out, "devctl")
	if strings.Contains(out.String(), "did not run") {
		t.Errorf("a run with no skips must not print a skip section; got:\n%s", out.String())
	}
}

type io_Discard struct{}

func (io_Discard) Write(p []byte) (int, error) { return len(p), nil }

func TestPackagePatternDefaultsSoTheModuleRootIsNotTested(t *testing.T) {
	// apps/platform's module root has no Go files; without ./... go test
	// reports "setup failed" and the report says 0 passed, 0 skipped.
	got := withPackagePattern([]string{"-count=1"})
	if len(got) != 2 || got[1] != "./..." {
		t.Errorf("got %v, want the pattern appended", got)
	}
	// An explicit package must win; appending would widen what the caller asked for.
	if got := withPackagePattern([]string{"-count=1", "./internal/skill/..."}); len(got) != 2 {
		t.Errorf("got %v, want the caller's packages untouched", got)
	}
}
