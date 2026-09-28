package main

import "testing"

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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ref, wait, err := parseCIStatusArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCIStatusArgs(%v) = nil error, want a usage error", tc.args)
				}
				return
			}
			if err != nil || ref != tc.wantRef || wait != tc.wantWait {
				t.Fatalf("parseCIStatusArgs(%v) = %q, %v, %v; want %q, %v, nil",
					tc.args, ref, wait, err, tc.wantRef, tc.wantWait)
			}
		})
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
