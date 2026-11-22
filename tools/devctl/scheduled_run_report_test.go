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

type workflowJobs struct {
	names             []string
	needs             map[string][]string
	conditions        map[string]string
	current, reporter string
	inNeeds           bool
}

func readWorkflowJobs(workflow string) workflowJobs {
	jobs := workflowJobs{needs: map[string][]string{}, conditions: map[string]string{}}
	_, body, _ := strings.Cut(workflow, "\njobs:\n")
	for _, line := range strings.Split(body, "\n") {
		jobs.read(line)
	}
	return jobs
}

func (jobs *workflowJobs) read(line string) {
	if match := ciJobHeader.FindStringSubmatch(line); match != nil {
		jobs.current, jobs.inNeeds = match[1], false
		jobs.names = append(jobs.names, jobs.current)
		return
	}
	switch match := neededJob.FindStringSubmatch(line); {
	case line == "    needs:":
		jobs.inNeeds = true
	case jobs.inNeeds && match != nil:
		jobs.needs[jobs.current] = append(jobs.needs[jobs.current], match[1])
	case strings.HasPrefix(line, "    "):
		jobs.inNeeds = jobs.inNeeds && strings.HasPrefix(line, "      ")
	}
	if condition, ok := strings.CutPrefix(line, "    if: "); ok {
		jobs.conditions[jobs.current] = condition
	}
	if strings.HasSuffix(line, scheduledRunReporter) {
		jobs.reporter = jobs.current
	}
}

func scheduledRunReportGaps(workflow string) []string {
	if !scheduleTrigger.MatchString(workflow) || strings.Contains(workflow, "gh issue create") {
		return nil
	}
	jobs := readWorkflowJobs(workflow)
	reporter := jobs.reporter
	if reporter == "" {
		return []string{"no job reports a failed scheduled run"}
	}
	var gaps []string
	if !strings.Contains(jobs.conditions[reporter], "always()") {
		gaps = append(gaps, reporter+" is skipped whenever a job it waits for fails")
	}
	for _, other := range jobs.names {
		if other != reporter && !slices.Contains(jobs.needs[reporter], other) {
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
