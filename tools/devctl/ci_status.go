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

const (
	ciHTTPTimeout                 = 30 * time.Second
	ciPollInterval                = 30 * time.Second
	ciPollIntervalUnauthenticated = 90 * time.Second
	ciStalledWait                 = 60 * time.Minute
	ciNoRunsWait                  = 5 * time.Minute
)

type workflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type workflowJob struct {
	ID         int64  `json:"id"`
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
	pause func(time.Duration)
}

type githubHTTPError struct {
	path   string
	status int
	detail string
}

func (e githubHTTPError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d%s", e.path, e.status, e.detail)
}

type checkAnnotation struct {
	Level   string `json:"annotation_level"`
	Message string `json:"message"`
}

const (
	ciAnnotationsPerJob    = 5
	ciAnnotationLinesShown = 8
	ciRunnerExitAnnotation = "Process completed with exit code"
)

func githubMayAnswerLater(err error) bool {
	var refused githubHTTPError
	if errors.As(err, &refused) {
		return refused.status >= http.StatusInternalServerError
	}
	return err != nil
}

func (c githubClient) wait(d time.Duration) {
	if c.pause != nil {
		c.pause(d)
		return
	}
	time.Sleep(d)
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

func ciStatus(root string, args []string, out io.Writer) (int, error) {
	ref, wait, err := parseCIStatusArgs(args)
	if err != nil {
		return 2, err
	}
	sha, err := resolveCommitSHA(root, ref)
	if err != nil {
		return 2, err
	}
	client, err := newGithubClientForOrigin(root)
	if err != nil {
		return 2, err
	}
	if !wait {
		return reportCIStatusOnce(client, sha, out)
	}
	return pollCIStatusUntilDecided(client, sha, out)
}

func parseCIStatusArgs(args []string) (ref string, wait bool, err error) {
	usage := errors.New("usage: devctl ci-status [ref | --sha ref] [--wait]")
	ref = "HEAD"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--wait":
			wait = true
		case arg == "--sha":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", false, usage
			}
		case strings.HasPrefix(arg, "--sha="):
			ref = strings.TrimPrefix(arg, "--sha=")
			if ref == "" {
				return "", false, usage
			}
		case strings.HasPrefix(arg, "-"):
			return "", false, usage
		default:
			ref = arg
		}
	}
	return ref, wait, nil
}

func resolveCommitSHA(root, ref string) (string, error) {
	sha, err := gitOutput(root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func newGithubClientForOrigin(root string) (githubClient, error) {
	remote, err := gitOutput(root, "remote", "get-url", "origin")
	if err != nil {
		return githubClient{}, err
	}
	repo, err := githubRepoFromRemote(strings.TrimSpace(remote))
	if err != nil {
		return githubClient{}, err
	}
	return githubClient{repo: repo, token: githubToken(root), http: &http.Client{Timeout: ciHTTPTimeout}}, nil
}

func reportCIStatusOnce(client githubClient, sha string, out io.Writer) (int, error) {
	runs, err := client.runsFor(sha)
	if err != nil {
		return 2, err
	}
	code, verdict := ciVerdict(runs)
	client.report(out, sha, verdict, runs)
	return code, nil
}

func pollCIStatusUntilDecided(client githubClient, sha string, out io.Writer) (int, error) {
	interval := ciPollInterval
	if client.token == "" {
		interval = ciPollIntervalUnauthenticated
	}

	started := time.Now()
	for {
		runs, err := client.runsFor(sha)
		if err != nil {
			if !githubMayAnswerLater(err) || time.Since(started) > ciStalledWait {
				return 2, err
			}
			fmt.Fprintf(out, "GitHub did not answer (%v); asking again in %s\n", err, interval)
			client.wait(interval)
			continue
		}
		code, verdict := ciVerdict(runs)
		waitedOut := time.Since(started) > ciStalledWait || (code == ciNoRuns && time.Since(started) > ciNoRunsWait)
		if (code != ciPending && code != ciNoRuns) || waitedOut {
			client.report(out, sha, verdict, runs)
			return code, nil
		}
		client.wait(interval)
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
		return githubHTTPError{path: path, status: response.StatusCode, detail: detail}
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

func (c githubClient) report(out io.Writer, sha, verdict string, runs []workflowRun) {
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
			fmt.Fprintf(out, "    jobs unavailable: %v\n", err)
			continue
		}
		c.reportJobs(out, page.Jobs)
	}
}

func (c githubClient) reportJobs(out io.Writer, jobs []workflowJob) {
	var skipped []string
	for _, job := range jobs {
		switch job.Conclusion {
		case "success", "neutral":
		case "skipped":
			skipped = append(skipped, job.Name)
		default:
			fmt.Fprintf(out, "    %s %s: %s\n", orDash(job.Conclusion), job.Name, strings.Join(failedSteps(job), " | "))
			c.reportFailureAnnotations(out, job)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(out, "    did not run: %s\n", strings.Join(skipped, ", "))
	}
}

func failedSteps(job workflowJob) []string {
	var steps []string
	for _, step := range job.Steps {
		if step.Conclusion == "failure" || step.Conclusion == "cancelled" {
			steps = append(steps, step.Name)
		}
	}
	return steps
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (c githubClient) reportFailureAnnotations(out io.Writer, job workflowJob) {
	var annotations []checkAnnotation
	if err := c.get(fmt.Sprintf("/check-runs/%d/annotations", job.ID), &annotations); err != nil {
		fmt.Fprintf(out, "      annotations unavailable: %v\n", err)
		return
	}
	shown := 0
	for _, annotation := range annotations {
		if annotation.Level != "failure" || strings.HasPrefix(annotation.Message, ciRunnerExitAnnotation) {
			continue
		}
		if shown == ciAnnotationsPerJob {
			fmt.Fprintf(out, "      … more annotations on the run page\n")
			return
		}
		shown++
		lines := strings.Split(strings.TrimSpace(annotation.Message), "\n")
		if len(lines) > ciAnnotationLinesShown {
			lines = append(lines[:ciAnnotationLinesShown], "…")
		}
		fmt.Fprintf(out, "      %s\n", strings.Join(lines, "\n      "))
	}
}
