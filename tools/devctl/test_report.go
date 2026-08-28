package main

// test-report runs `go test -json` and prints the number the plain summary
// leaves out: how many tests removed themselves, and why.
//
// 02:PORT-004. The failure this exists to catch is not a red build, it is a
// green one. apps/platform has 287 tests that skip when the database URL is
// unset, and `go test ./...` still prints ok for every package — a developer
// without a container environment reads a full green screen for 63% of the
// suite and nothing tells them. The skip messages themselves are honest; the
// summary is where the information disappears.
//
// This does not make skipping fail. That is SKILLHUB_REQUIRE_DB's job, in each
// TestMain. Reporting and enforcing are separate on purpose: a report that
// exits non-zero would just get piped to /dev/null.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

type testSummary struct {
	Passed  int
	Skipped int
	Failed  int
	// Reasons maps a normalised skip message to how many tests gave it.
	Reasons map[string]int
}

// skipMessage matches the "    file_test.go:171: reason" line that t.Skip
// writes. The reason is what we group by; the file and line are what make two
// identical reasons look like different ones, so they come off.
var skipMessage = regexp.MustCompile(`^\s*[\w./-]+\.go:\d+:\s*(.+)$`)

// summarize reads `go test -json` events. Output events are echoed to raw so
// the caller still sees ordinary test output; only the tallies are new.
func summarize(r io.Reader, raw io.Writer) (testSummary, error) {
	s := testSummary{Reasons: map[string]int{}}
	// Output arrives before the pass/fail/skip event for the same test, so both
	// the skip reason and the failure text have to be held until the verdict
	// lands. -json implies -v, so echoing every line as it arrives would bury
	// the summary under thousands of PASS lines: a passing test's output is
	// dropped, a failing one's is printed.
	lastMessage := map[string]string{}
	held := map[string][]string{}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Not an event — go test writes build errors as plain text.
			fmt.Fprintln(raw, string(line))
			continue
		}
		switch ev.Action {
		case "output":
			if ev.Test == "" {
				// Package level: "ok pkg 0.4s", FAIL lines, build errors.
				fmt.Fprint(raw, ev.Output)
				continue
			}
			key := ev.Package + "\x00" + ev.Test
			held[key] = append(held[key], ev.Output)
			if m := skipMessage.FindStringSubmatch(strings.TrimRight(ev.Output, "\n")); m != nil {
				lastMessage[key] = strings.TrimSpace(m[1])
			}
		case "pass":
			if ev.Test != "" {
				s.Passed++
				delete(held, ev.Package+"\x00"+ev.Test)
			}
		case "fail":
			if ev.Test != "" {
				s.Failed++
				key := ev.Package + "\x00" + ev.Test
				for _, line := range held[key] {
					fmt.Fprint(raw, line)
				}
				delete(held, key)
			}
		case "skip":
			if ev.Test == "" {
				// A whole package with no test files; not a skipped test.
				continue
			}
			s.Skipped++
			key := ev.Package + "\x00" + ev.Test
			delete(held, key)
			reason := lastMessage[key]
			if reason == "" {
				reason = "(no reason given)"
			}
			s.Reasons[reason]++
		}
	}
	return s, scanner.Err()
}

func (s testSummary) write(out io.Writer, label string) {
	fmt.Fprintf(out, "\n%s: %d passed, %d skipped, %d failed\n", label, s.Passed, s.Skipped, s.Failed)
	if s.Skipped == 0 {
		return
	}
	type row struct {
		reason string
		n      int
	}
	rows := make([]row, 0, len(s.Reasons))
	for reason, n := range s.Reasons {
		rows = append(rows, row{reason, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].reason < rows[j].reason
	})
	fmt.Fprintf(out, "%d test(s) did not run, by reason:\n", s.Skipped)
	for _, r := range rows {
		fmt.Fprintf(out, "  %5d  %s\n", r.n, r.reason)
	}
	fmt.Fprintln(out, "A skipped test asserts nothing. Set the environment it names, or accept that this run did not check it.")
}

// withPackagePattern appends ./... unless the caller named packages. Without
// it `go test -json -count=1` tests the module root, which in apps/platform
// holds no Go files and reports "setup failed" — a red build that says nothing
// about the tests.
func withPackagePattern(args []string) []string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return args
		}
	}
	return append(append([]string{}, args...), "./...")
}

// testReport runs the suite in dir and returns go test's own exit code so the
// task fails exactly when go test would have.
func testReport(root, dir string, args []string, out io.Writer) (int, error) {
	cmd := exec.Command("go", append([]string{"test", "-json"}, withPackagePattern(args)...)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	s, sumErr := summarize(pipe, out)
	waitErr := cmd.Wait()
	if sumErr != nil {
		return 1, sumErr
	}
	label := strings.TrimPrefix(strings.TrimPrefix(dir, root), string(os.PathSeparator))
	if label == "" {
		label = dir
	}
	s.write(out, label)
	if waitErr != nil {
		if exit, ok := waitErr.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 1, waitErr
	}
	return 0, nil
}
