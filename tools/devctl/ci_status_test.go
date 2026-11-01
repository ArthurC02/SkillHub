package main

import (
	"strings"
	"testing"
	"time"
)

func TestCIVerdictIsGreenOnlyWhenEveryRunFinishedWithoutFailing(t *testing.T) {
	t.Parallel()
	done := func(conclusion string) workflowRun {
		return workflowRun{Name: conclusion, Status: "completed", Conclusion: conclusion}
	}
	running := workflowRun{Name: "running", Status: "in_progress"}
	cases := []struct {
		name string
		runs []workflowRun
		want int
	}{
		{"no runs", nil, ciNoRuns},
		{"one success", []workflowRun{done("success")}, ciGreen},
		{"success, skipped and neutral", []workflowRun{done("success"), done("skipped"), done("neutral")}, ciGreen},
		{"still running", []workflowRun{done("success"), running}, ciPending},
		{"queued", []workflowRun{{Status: "queued"}}, ciPending},
		{"a failure", []workflowRun{done("success"), done("failure")}, ciRed},
		{"a failure while another still runs", []workflowRun{running, done("failure")}, ciRed},
		{"cancelled is not verified", []workflowRun{done("cancelled")}, ciRed},
		{"timed out", []workflowRun{done("timed_out")}, ciRed},
		{"startup failure", []workflowRun{done("startup_failure")}, ciRed},
	}
	for _, tc := range cases {
		if got, _ := ciVerdict(tc.runs); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestParseCIStatusArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		args     []string
		wantRef  string
		wantWait bool
		wantErr  bool
	}{
		{"no arguments default to HEAD without waiting", nil, "HEAD", false, false},
		{"a bare ref is used as given", []string{"abc123"}, "abc123", false, false},
		{"--wait alone keeps the default ref", []string{"--wait"}, "HEAD", true, false},
		{"a ref and --wait together", []string{"abc123", "--wait"}, "abc123", true, false},
		{"an unknown flag is rejected", []string{"--bogus"}, "", false, true},
		{"--sha names the ref", []string{"--sha", "abc123", "--wait"}, "abc123", true, false},
		{"--sha= names the ref", []string{"--sha=abc123"}, "abc123", false, false},
		{"--sha without a value is rejected", []string{"--sha"}, "", false, true},
		{"--sha followed by a flag is rejected", []string{"--sha", "--wait"}, "", false, true},
		{"--sha= with an empty value is rejected", []string{"--sha="}, "", false, true},
		{"--timing is not a ref and does not wait", []string{"--timing"}, "HEAD", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			options, err := parseCIStatusArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCIStatusArgs(%v) = nil error, want a usage error", tc.args)
				}
				return
			}
			if err != nil || options.ref != tc.wantRef || options.wait != tc.wantWait {
				t.Fatalf("parseCIStatusArgs(%v) = %+v, %v; want ref %q, wait %v, nil",
					tc.args, options, err, tc.wantRef, tc.wantWait)
			}
		})
	}
}

func TestTimingIsOffUnlessAsked(t *testing.T) {
	t.Parallel()
	off, _ := parseCIStatusArgs([]string{"abc123", "--wait"})
	on, _ := parseCIStatusArgs([]string{"--timing", "abc123"})
	if off.timing || !on.timing || on.ref != "abc123" {
		t.Fatalf("timing: off=%+v on=%+v", off, on)
	}
}

func TestJobTimingsListTheLongestJobFirstNameQueueWaitsOfAMinuteAndSkipUnfinishedOrSkippedOnes(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return start.Add(time.Duration(seconds) * time.Second) }
	jobs := []workflowJob{
		{Name: "quick", CreatedAt: at(-59), StartedAt: start, CompletedAt: at(42)},
		{Name: "waited", CreatedAt: at(-150), StartedAt: start, CompletedAt: at(30)},
		{Name: "running", StartedAt: start},
		{Name: "skipped", StartedAt: start, CompletedAt: start},
		{Name: "skipped by a clock skew", StartedAt: start, CompletedAt: start.Add(-time.Second)},
		{Name: "slow", StartedAt: start, CompletedAt: start.Add(12*time.Minute + 3*time.Second + 600*time.Millisecond), Steps: []workflowStep{
			{Name: "checkout", StartedAt: start, CompletedAt: at(4)},
			{Name: "tests", StartedAt: at(4), CompletedAt: at(600)},
			{Name: "lint", StartedAt: at(600), CompletedAt: at(700)},
			{Name: "upload", StartedAt: at(700), CompletedAt: at(720)},
			{Name: "still running", StartedAt: at(720)},
		}},
	}
	got := jobTimings(jobs)
	want := []string{
		"took    12m4s  slow",
		"        9m56s    tests",
		"        1m40s    lint",
		"          20s    upload",
		"took      42s  quick",
		"took      30s  waited  (after 2m30s in the queue)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("jobTimings() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestGitHubRepoIsReadFromTheOriginURL(t *testing.T) {
	t.Parallel()
	for remote, want := range map[string]string{
		"https://github.com/ArthurC02/SkillHub.git": "ArthurC02/SkillHub",
		"https://github.com/ArthurC02/SkillHub":     "ArthurC02/SkillHub",
		"git@github.com:ArthurC02/SkillHub.git":     "ArthurC02/SkillHub",
		"ssh://git@github.com/ArthurC02/SkillHub/":  "ArthurC02/SkillHub",
	} {
		if got, err := githubRepoFromRemote(remote); err != nil || got != want {
			t.Errorf("githubRepoFromRemote(%q) = %q, %v; want %q", remote, got, err, want)
		}
	}
	if _, err := githubRepoFromRemote("https://gitlab.com/a/b.git"); err == nil {
		t.Error("a non-GitHub remote was accepted")
	}
}
