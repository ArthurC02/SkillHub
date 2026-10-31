package main

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestToolchainFreshnessNotesOnlyCooledNewerReleasesAndFailedLookups(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		release toolRelease
		err     error
		want    []string
	}{
		{"pin is the latest release", toolRelease{"3.54.0", now.Add(-30 * 24 * time.Hour)}, nil, nil},
		{"newer release exactly at the cooldown", toolRelease{"3.55.0", now.Add(-7 * 24 * time.Hour)}, nil,
			[]string{"toolchain task: pinned 3.54.0, 3.55.0 released 2026-10-24; tools/toolchain.yaml is not watched by Dependabot"}},
		{"newer release one second inside the cooldown", toolRelease{"3.55.0", now.Add(-7*24*time.Hour + time.Second)}, nil, nil},
		{"lookup failed", toolRelease{}, errors.New("503 Service Unavailable"),
			[]string{"toolchain task: could not look up the latest release (503 Service Unavailable)"}},
	}
	pins := map[string]string{}
	for key := range toolchainReleaseSources {
		pins[key] = "pinned"
	}
	pins["task"] = "3.54.0"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lookup := func(source releaseSource) (toolRelease, error) {
				if source.name != "github.com/go-task/task/v3" {
					return toolRelease{version: "pinned"}, nil
				}
				return c.release, c.err
			}
			if notes := toolchainFreshnessNotes(pins, lookup, now); !slices.Equal(notes, c.want) {
				t.Fatalf("got %q, want %q", notes, c.want)
			}
		})
	}
}

type registryReplies map[string]string

func (replies registryReplies) RoundTrip(request *http.Request) (*http.Response, error) {
	body, found := replies[request.URL.String()]
	status := http.StatusOK
	if !found {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestLatestReleaseReadsEachRegistry(t *testing.T) {
	t.Parallel()
	replies := registryReplies{
		"https://proxy.golang.org/github.com/docker/cli/@latest": `{"Version":"v29.9.2+incompatible","Time":"2026-10-20T11:24:07Z"}`,
		"https://pypi.org/pypi/uv/json": `{"info":{"version":"0.13.1"},"releases":{"0.13.0":[{"upload_time_iso_8601":"2026-09-01T00:00:00Z"}],
			"0.13.1":[{"upload_time_iso_8601":"2026-10-02T03:04:05.123456Z"}]}}`,
		"https://pypi.org/pypi/yanked/json":                                           `{"info":{"version":"1.0.0"},"releases":{"1.0.0":[]}}`,
		"https://registry.npmjs.org/@redocly/cli":                                     `{"dist-tags":{"latest":"2.72.0"},"time":{"created":"2020-01-01T00:00:00.000Z","2.72.0":"2026-10-10T10:00:00.000Z"}}`,
		"https://api.github.com/repos/OpenAPITools/openapi-generator/releases/latest": `{"tag_name":"v7.26.0","published_at":"2026-10-06T09:33:32Z"}`,
	}
	latest := latestReleaseFrom(&http.Client{Transport: replies})
	cases := []struct {
		name, wantVersion, wantReleased, wantErr string
		source                                   releaseSource
	}{
		{"go proxy drops the v and +incompatible", "29.9.2", "2026-10-20T11:24:07Z", "", releaseSource{registryGoProxy, "github.com/docker/cli"}},
		{"pypi takes the upload time of the latest version", "0.13.1", "2026-10-02T03:04:05.123456Z", "", releaseSource{registryPyPI, "uv"}},
		{"pypi latest version without files", "", "", "pypi lists no files for yanked 1.0.0", releaseSource{registryPyPI, "yanked"}},
		{"npm takes the time of the latest dist-tag", "2.72.0", "2026-10-10T10:00:00Z", "", releaseSource{registryNPM, "@redocly/cli"}},
		{"github release drops the v", "7.26.0", "2026-10-06T09:33:32Z", "", releaseSource{registryGitHubReleases, "OpenAPITools/openapi-generator"}},
		{"registry answers not found", "", "", "Not Found", releaseSource{registryGoProxy, "example.com/gone"}},
		{"unknown registry", "", "", `unknown registry "maven"`, releaseSource{"maven", "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			release, err := latest(c.source)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("got %v, want an error containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if release.version != c.wantVersion || release.released.Format(time.RFC3339Nano) != c.wantReleased {
				t.Fatalf("got %s at %s, want %s at %s", release.version, release.released.Format(time.RFC3339Nano), c.wantVersion, c.wantReleased)
			}
		})
	}
}

func TestEveryToolchainPinIsEitherLookedUpOrFollowsAWatchedManifest(t *testing.T) {
	t.Parallel()
	pins, err := parseToolchain(filepath.Join("..", "toolchain.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) == 0 {
		t.Fatal("tools/toolchain.yaml listed no tools")
	}
	for key := range pins {
		_, lookedUp := toolchainReleaseSources[key]
		follows := slices.Contains(toolchainPinsFollowingAWatchedManifest, key)
		if lookedUp == follows {
			t.Errorf("%s: looked up %v, follows a watched manifest %v; exactly one must hold", key, lookedUp, follows)
		}
	}
	for key := range toolchainReleaseSources {
		if _, pinned := pins[key]; !pinned {
			t.Errorf("%s is looked up but not pinned in tools/toolchain.yaml", key)
		}
	}
}
