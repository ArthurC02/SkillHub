package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixedGitHubResponse string

func (body fixedGitHubResponse) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

func TestTheCIReportNamesFailedStepsAndJobsThatDidNotRun(t *testing.T) {
	t.Parallel()
	jobs := fixedGitHubResponse(`{"jobs":[
		{"name":"lint","conclusion":"success"},
		{"name":"e2e","conclusion":"skipped"},
		{"name":"test","conclusion":"failure","steps":[
			{"name":"setup","conclusion":"success"},
			{"name":"go test","conclusion":"failure"},
			{"name":"upload","conclusion":"cancelled"}]}]}`)
	client := githubClient{repo: "owner/repo", http: &http.Client{Transport: jobs}}
	runs := []workflowRun{{ID: 7, Name: "CI", Status: "completed", Conclusion: "failure", HTMLURL: "https://example.invalid/7"}}
	var out strings.Builder
	if err := client.report(&out, "0123456789abcdef", "red", runs); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"    failure test: go test | upload\n", "    did not run: e2e\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "lint") || strings.Contains(got, "setup") {
		t.Errorf("a passing job or step was reported as a problem:\n%s", got)
	}
}
