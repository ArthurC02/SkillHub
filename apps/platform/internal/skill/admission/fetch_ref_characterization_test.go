package ingest

import (
	"context"
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
