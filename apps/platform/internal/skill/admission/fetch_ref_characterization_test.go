package ingest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type codeloadStub map[string]string

func (s codeloadStub) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := s[req.URL.String()]
	status := http.StatusOK
	if !ok {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestAGitHubSourceReportsTheRefItWasDownloadedAt(t *testing.T) {
	original := devClient
	t.Cleanup(func() { devClient = original })
	devClient = &http.Client{Transport: codeloadStub{
		"https://codeload.github.com/o/r/zip/refs/heads/master": "master-zip",
		"https://codeload.github.com/o/r/zip/refs/heads/v1.2":   "tag-zip",
	}}
	f := &URLFetcher{Allowed: DefaultAllowedHosts(), AllowInsecure: true}

	for _, tc := range []struct {
		name, url, wantData, wantRef string
	}{
		{"the repository root falls back to master", "https://github.com/o/r", "master-zip", "master"},
		{"a tree URL names its branch", "https://github.com/o/r/tree/v1.2", "tag-zip", "v1.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, ref, err := f.Fetch(context.Background(), tc.url)

			if err != nil || string(data) != tc.wantData || ref != tc.wantRef {
				t.Fatalf("data=%q ref=%q err=%v, want %q at %q", data, ref, err, tc.wantData, tc.wantRef)
			}
		})
	}
}

type statusStub map[string]int

func (s statusStub) RoundTrip(req *http.Request) (*http.Response, error) {
	status, ok := s[req.URL.String()]
	if !ok {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

func TestAGitHubRootReportsWhyItsDefaultBranchFailedRatherThanTheFallbacksAbsence(t *testing.T) {
	original := devClient
	t.Cleanup(func() { devClient = original })
	devClient = &http.Client{Transport: statusStub{
		"https://codeload.github.com/o/r/zip/refs/heads/main": http.StatusServiceUnavailable,
	}}
	f := &URLFetcher{Allowed: DefaultAllowedHosts(), AllowInsecure: true}

	_, _, err := f.Fetch(context.Background(), "https://github.com/o/r")

	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %v, want the main branch's HTTP 503, not master's 404", err)
	}
}

func TestAGitHubRootWithNeitherDefaultBranchSaysNothingIsThere(t *testing.T) {
	original := devClient
	t.Cleanup(func() { devClient = original })
	devClient = &http.Client{Transport: statusStub{}}
	f := &URLFetcher{Allowed: DefaultAllowedHosts(), AllowInsecure: true}

	_, _, err := f.Fetch(context.Background(), "https://github.com/o/r")

	if !errors.Is(err, ErrFetch) || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want an ErrFetch naming HTTP 404", err)
	}
}
