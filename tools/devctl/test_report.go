package main

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

	Reasons map[string]int
}

var skipMessage = regexp.MustCompile(`^\s*[\w./-]+\.go:\d+:\s*(.+)$`)

func summarize(r io.Reader, raw io.Writer) (testSummary, error) {
	s := testSummary{Reasons: map[string]int{}}

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

			fmt.Fprintln(raw, string(line))
			continue
		}
		switch ev.Action {
		case "output":
			if ev.Test == "" {

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

func withPackagePattern(args []string) []string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return args
		}
	}
	return append(append([]string{}, args...), "./...")
}

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
