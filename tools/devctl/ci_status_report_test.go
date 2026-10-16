package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
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
