package main

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type githubStub func(path string) (int, string)

func (stub githubStub) RoundTrip(request *http.Request) (*http.Response, error) {
	status, body := stub(request.URL.RequestURI())
	header := http.Header{}
	if status == http.StatusForbidden {
		header.Set("X-RateLimit-Remaining", "0")
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestAVerdictStandsWhenItsJobDetailCannotBeRead(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, conclusion string
		want             int
	}{
		{"green", "success", ciGreen},
		{"red", "failure", ciRed},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			client := githubClient{repo: "o/r", http: &http.Client{Transport: githubStub(func(path string) (int, string) {
				if strings.Contains(path, "/jobs") {
					return http.StatusForbidden, `{"message":"API rate limit exceeded"}`
				}
				return http.StatusOK, `{"workflow_runs":[{"id":7,"name":"CI","status":"completed","conclusion":"` + c.conclusion + `"}]}`
			})}}
			var out strings.Builder
			code, err := reportCIStatusOnce(client, strings.Repeat("a", 40), &out)
			if err != nil || code != c.want {
				t.Fatalf("code, err = %d, %v; want %d, nil", code, err, c.want)
			}
			if !strings.Contains(out.String(), "jobs unavailable: GET /actions/runs/7/jobs?per_page=100: HTTP 403 (rate limit exhausted") {
				t.Fatalf("the unreadable job detail was not named:\n%s", out.String())
			}
		})
	}
}

func TestARunListThatCannotBeReadIsNeitherGreenNorRed(t *testing.T) {
	t.Parallel()
	client := githubClient{repo: "o/r", http: &http.Client{Transport: githubStub(func(string) (int, string) {
		return http.StatusForbidden, `{}`
	})}}
	code, err := reportCIStatusOnce(client, strings.Repeat("a", 40), io.Discard)
	if err == nil || exitCode(code, err) != 2 {
		t.Fatalf("exit = %d (err %v), want 2: not the red verdict's 1", exitCode(code, err), err)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	failed := errors.New("failed")
	for _, c := range []struct {
		name string
		code int
		err  error
		want int
	}{
		{"success", 0, nil, 0},
		{"a verdict without an error", ciPending, nil, ciPending},
		{"an error that names its own code", 2, failed, 2},
		{"an error with no code", 0, failed, 1},
	} {
		if got := exitCode(c.code, c.err); got != c.want {
			t.Errorf("%s: exitCode(%d, %v) = %d, want %d", c.name, c.code, c.err, got, c.want)
		}
	}
}

func TestARedRunNamesTheFailingAssertionsFromItsAnnotations(t *testing.T) {
	t.Parallel()
	failure := func(message string) string {
		return `{"annotation_level":"failure","message":` + strconv.Quote(message) + `}`
	}
	annotations := []string{
		failure("Process completed with exit code 1."),
		`{"annotation_level":"warning","message":"The ubuntu-latest label will migrate"}`,
		failure("--- FAIL: TestHalt (0.5s)\n    halt_test.go:133: refused with the English sentence"),
	}
	for i := 0; i < ciAnnotationsPerJob; i++ {
		annotations = append(annotations, failure("another failure"))
	}
	client := githubClient{repo: "o/r", http: &http.Client{Transport: githubStub(func(path string) (int, string) {
		switch {
		case strings.Contains(path, "/annotations"):
			return http.StatusOK, "[" + strings.Join(annotations, ",") + "]"
		case strings.Contains(path, "/jobs"):
			return http.StatusOK, `{"jobs":[{"id":42,"name":"platform","conclusion":"failure","steps":[{"name":"Platform tests","conclusion":"failure"}]}]}`
		}
		return http.StatusOK, `{"workflow_runs":[{"id":7,"name":"CI","status":"completed","conclusion":"failure"}]}`
	})}}

	var out strings.Builder
	code, err := reportCIStatusOnce(client, strings.Repeat("a", 40), &out)

	if err != nil || code != ciRed {
		t.Fatalf("code, err = %d, %v; want red", code, err)
	}
	report := out.String()
	for _, want := range []string{"failure platform: Platform tests", "--- FAIL: TestHalt", "halt_test.go:133: refused", "… more annotations"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	for _, noise := range []string{"Process completed with exit code", "ubuntu-latest"} {
		if strings.Contains(report, noise) {
			t.Errorf("report carries the noise %q:\n%s", noise, report)
		}
	}
	if got := strings.Count(report, "another failure"); got != ciAnnotationsPerJob-1 {
		t.Errorf("showed %d extra failures, want %d (the cap less the one named failure)", got, ciAnnotationsPerJob-1)
	}
}

func TestWaitingAsksAgainWhenGitHubFailsOnItsSideButNotWhenItRefuses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		first    int
		wantCode int
		wantErr  bool
	}{
		{"a 502 is asked again", http.StatusBadGateway, ciGreen, false},
		{"a 504 is asked again", http.StatusGatewayTimeout, ciGreen, false},
		{"a 404 is not", http.StatusNotFound, 2, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			paused := 0
			client := githubClient{repo: "o/r", token: "t", pause: func(time.Duration) { paused++ },
				http: &http.Client{Transport: githubStub(func(path string) (int, string) {
					if strings.Contains(path, "/jobs") {
						return http.StatusOK, `{"jobs":[]}`
					}
					calls++
					if calls == 1 {
						return c.first, `{}`
					}
					return http.StatusOK, `{"workflow_runs":[{"id":7,"name":"CI","status":"completed","conclusion":"success"}]}`
				})}}

			code, err := pollCIStatusUntilDecided(client, strings.Repeat("a", 40), io.Discard)

			if code != c.wantCode || (err != nil) != c.wantErr {
				t.Fatalf("code, err = %d, %v; want %d, error %v", code, err, c.wantCode, c.wantErr)
			}
			if c.wantErr == (paused > 0) {
				t.Errorf("paused %d times; a refusal must not wait and a server failure must", paused)
			}
		})
	}
}
