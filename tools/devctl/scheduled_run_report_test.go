package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const scheduledRunReporter = "uses: ./.github/actions/report-scheduled-run"

var (
	scheduleTrigger = regexp.MustCompile(`(?m)^  schedule:$`)
	neededJob       = regexp.MustCompile(`^ {6}- ([a-z0-9-]+)$`)
)

func scheduledRunReportGaps(workflow string) []string {
	if !scheduleTrigger.MatchString(workflow) || strings.Contains(workflow, "gh issue create") {
		return nil
	}
	var jobs []string
	needs := map[string][]string{}
	conditions := map[string]string{}
	inJobs, inNeeds := false, false
	job, reporter := "", ""
	for _, line := range strings.Split(workflow, "\n") {
		if line == "jobs:" {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		if match := ciJobHeader.FindStringSubmatch(line); match != nil {
			job, inNeeds = match[1], false
			jobs = append(jobs, job)
			continue
		}
		switch match := neededJob.FindStringSubmatch(line); {
		case line == "    needs:":
			inNeeds = true
		case inNeeds && match != nil:
			needs[job] = append(needs[job], match[1])
		case strings.HasPrefix(line, "    "):
			inNeeds = inNeeds && strings.HasPrefix(line, "      ")
		}
		if condition, ok := strings.CutPrefix(line, "    if: "); ok {
			conditions[job] = condition
		}
		if strings.HasSuffix(line, scheduledRunReporter) {
			reporter = job
		}
	}
	if reporter == "" {
		return []string{"no job reports a failed scheduled run"}
	}
	var gaps []string
	if !strings.Contains(conditions[reporter], "always()") {
		gaps = append(gaps, reporter+" is skipped whenever a job it waits for fails")
	}
	for _, other := range jobs {
		if other != reporter && !slices.Contains(needs[reporter], other) {
			gaps = append(gaps, reporter+" does not wait for "+other)
		}
	}
	return gaps
}

func TestEveryScheduledWorkflowReportsItsFailures(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	scheduled := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		workflow := strings.ReplaceAll(string(data), "\r\n", "\n")
		if scheduleTrigger.MatchString(workflow) {
			scheduled++
		}
		for _, gap := range scheduledRunReportGaps(workflow) {
			t.Errorf("%s: %s, so a red scheduled run tells no one", filepath.Base(path), gap)
		}
	}
	if scheduled == 0 {
		t.Fatal("found no scheduled workflow")
	}
}

func TestScheduledRunReportGaps(t *testing.T) {
	t.Parallel()
	const head = "on:\n  push:\n  schedule:\n    - cron: '0 0 * * 1'\njobs:\n" +
		"  build:\n    runs-on: ubuntu-latest\n" +
		"  scan:\n    needs: [build]\n    runs-on: ubuntu-latest\n"
	reporter := func(condition string, needs ...string) string {
		job := "  report:\n    needs:\n"
		for _, need := range needs {
			job += "      - " + need + "\n"
		}
		return job + "    if: " + condition + "\n    steps:\n      - " + scheduledRunReporter + "\n"
	}
	cases := []struct {
		name, workflow string
		want           []string
	}{
		{"a workflow without a schedule needs no report", "on:\n  push:\njobs:\n  build:\n    runs-on: ubuntu-latest\n", nil},
		{"a schedule that files its own issue", head + "    steps:\n      - run: gh issue create --title x\n", nil},
		{"a schedule with no report", head, []string{"no job reports a failed scheduled run"}},
		{"an action whose name only starts like the reporter", head + "  report:\n    if: ${{ always() }}\n    steps:\n      - " + scheduledRunReporter + "-v2\n",
			[]string{"no job reports a failed scheduled run"}},
		{"a report that waits for every job even when they fail", head + reporter("${{ always() }}", "build", "scan"), nil},
		{"a report skipped when a job fails", head + reporter("github.event_name == 'schedule'", "build", "scan"),
			[]string{"report is skipped whenever a job it waits for fails"}},
		{"a report that misses one job", head + reporter("${{ always() }}", "build"), []string{"report does not wait for scan"}},
		{"a report with no needs", head + reporter("${{ always() }}"),
			[]string{"report does not wait for build", "report does not wait for scan"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scheduledRunReportGaps(c.workflow); !reflect.DeepEqual(got, c.want) {
				t.Errorf("gaps = %q, want %q", got, c.want)
			}
		})
	}
}
