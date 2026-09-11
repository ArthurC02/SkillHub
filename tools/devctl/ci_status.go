package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	ciGreen   = 0
	ciRed     = 1
	ciPending = 3
	ciNoRuns  = 4
)

type workflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type workflowJob struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	Steps      []struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
	} `json:"steps"`
}

type githubClient struct {
	repo  string
	token string
	http  *http.Client
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

func ciStatus(root string, args []string, out io.Writer) (int, error) {
	ref, wait := "HEAD", false
	for _, arg := range args {
		switch {
		case arg == "--wait":
			wait = true
		case strings.HasPrefix(arg, "-"):
			return 2, errors.New("usage: devctl ci-status [ref] [--wait]")
		default:
			ref = arg
		}
	}
	sha, err := gitOutput(root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return 2, err
	}
	sha = strings.TrimSpace(sha)
	remote, err := gitOutput(root, "remote", "get-url", "origin")
	if err != nil {
		return 2, err
	}
	repo, err := githubRepoFromRemote(strings.TrimSpace(remote))
	if err != nil {
		return 2, err
	}
	client := githubClient{repo: repo, token: githubToken(root), http: &http.Client{Timeout: 30 * time.Second}}
	interval := 30 * time.Second
	if client.token == "" {
		interval = 90 * time.Second
	}

	started := time.Now()
	for {
		runs, err := client.runsFor(sha)
		if err != nil {
			return 2, err
		}
		code, verdict := ciVerdict(runs)
		waitedOut := time.Since(started) > 60*time.Minute || (code == ciNoRuns && time.Since(started) > 5*time.Minute)
		if !wait || (code != ciPending && code != ciNoRuns) || waitedOut {
			return code, client.report(out, sha, verdict, runs)
		}
		time.Sleep(interval)
	}
}

func ciVerdict(runs []workflowRun) (int, string) {
	if len(runs) == 0 {
		return ciNoRuns, "no workflow runs for this commit yet"
	}
	pending := false
	for _, run := range runs {
		if run.Status != "completed" {
			pending = true
			continue
		}
		switch run.Conclusion {
		case "success", "skipped", "neutral":
		default:
			return ciRed, "red"
		}
	}
	if pending {
		return ciPending, "pending"
	}
	return ciGreen, "green"
}

func githubRepoFromRemote(remote string) (string, error) {
	match := githubRemote.FindStringSubmatch(remote)
	if match == nil {
		return "", fmt.Errorf("origin %q is not a GitHub remote", remote)
	}
	return match[1] + "/" + match[2], nil
}

func githubToken(root string) string {
	cmd := exec.Command("git", "-C", root, "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "password="); ok {
			return value
		}
	}
	return ""
}

func (c githubClient) get(path string, into any) error {
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+c.repo+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail := ""
		if response.Header.Get("X-RateLimit-Remaining") == "0" {
			detail = fmt.Sprintf(" (rate limit exhausted until unix %s; unauthenticated calls get 60 an hour)", response.Header.Get("X-RateLimit-Reset"))
		}
		return fmt.Errorf("GET %s: HTTP %d%s", path, response.StatusCode, detail)
	}
	return json.NewDecoder(response.Body).Decode(into)
}

func (c githubClient) runsFor(sha string) ([]workflowRun, error) {
	var page struct {
		Runs []workflowRun `json:"workflow_runs"`
	}
	if err := c.get("/actions/runs?per_page=100&head_sha="+sha, &page); err != nil {
		return nil, err
	}
	sort.Slice(page.Runs, func(i, j int) bool { return page.Runs[i].Name < page.Runs[j].Name })
	return page.Runs, nil
}

func (c githubClient) report(out io.Writer, sha, verdict string, runs []workflowRun) error {
	fmt.Fprintf(out, "%s (%s): %s\n", sha[:7], sha, verdict)
	for _, run := range runs {
		fmt.Fprintf(out, "  %s: %s/%s %s\n", run.Name, run.Status, orDash(run.Conclusion), run.HTMLURL)
		if run.Status != "completed" {
			continue
		}
		var page struct {
			Jobs []workflowJob `json:"jobs"`
		}
		if err := c.get(fmt.Sprintf("/actions/runs/%d/jobs?per_page=100", run.ID), &page); err != nil {
			return err
		}
		var skipped []string
		for _, job := range page.Jobs {
			switch job.Conclusion {
			case "success", "neutral":
			case "skipped":
				skipped = append(skipped, job.Name)
			default:
				var steps []string
				for _, step := range job.Steps {
					if step.Conclusion == "failure" || step.Conclusion == "cancelled" {
						steps = append(steps, step.Name)
					}
				}
				fmt.Fprintf(out, "    %s %s: %s\n", orDash(job.Conclusion), job.Name, strings.Join(steps, " | "))
			}
		}
		if len(skipped) > 0 {
			fmt.Fprintf(out, "    did not run: %s\n", strings.Join(skipped, ", "))
		}
	}
	return nil
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
