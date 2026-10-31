package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	toolchainCooldown      = 7 * 24 * time.Hour
	toolchainLookupTimeout = 20 * time.Second
	registryGoProxy        = "go"
	registryPyPI           = "pypi"
	registryNPM            = "npm"
	registryGitHubReleases = "github"
)

type releaseSource struct{ registry, name string }

type toolRelease struct {
	version  string
	released time.Time
}

var toolchainReleaseSources = map[string]releaseSource{
	"task":              {registryGoProxy, "github.com/go-task/task/v3"},
	"uv":                {registryPyPI, "uv"},
	"docker_cli":        {registryGoProxy, "github.com/docker/cli"},
	"docker_compose":    {registryGoProxy, "github.com/docker/compose/v5"},
	"golangci_lint":     {registryGoProxy, "github.com/golangci/golangci-lint/v2"},
	"sqlc":              {registryGoProxy, "github.com/sqlc-dev/sqlc"},
	"redocly":           {registryNPM, "@redocly/cli"},
	"openapi_generator": {registryGitHubReleases, "OpenAPITools/openapi-generator"},
	"govulncheck":       {registryGoProxy, "golang.org/x/vuln"},
	"pip_audit":         {registryPyPI, "pip-audit"},
	"go_licenses":       {registryGoProxy, "github.com/google/go-licenses/v2"},
	toolZizmor:          {registryPyPI, toolZizmor},
}

var toolchainPinsFollowingAWatchedManifest = []string{
	"ogen", "datamodel_code_generator", toolPglite, "pglite_socket", "pglite_pgvector", "pglite_pgvector_extension",
}

func toolchainFreshnessNotes(pins map[string]string, latest func(releaseSource) (toolRelease, error), now time.Time) []string {
	keys := make([]string, 0, len(toolchainReleaseSources))
	for key := range toolchainReleaseSources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var notes []string
	for _, key := range keys {
		release, err := latest(toolchainReleaseSources[key])
		switch {
		case err != nil:
			notes = append(notes, fmt.Sprintf("toolchain %s: could not look up the latest release (%v)", key, err))
		case release.version != pins[key] && now.Sub(release.released) >= toolchainCooldown:
			notes = append(notes, fmt.Sprintf("toolchain %s: pinned %s, %s released %s; tools/toolchain.yaml is not watched by Dependabot",
				key, pins[key], release.version, release.released.Format(time.DateOnly)))
		}
	}
	return notes
}

func latestReleaseFrom(client *http.Client) func(releaseSource) (toolRelease, error) {
	return func(source releaseSource) (toolRelease, error) {
		switch source.registry {
		case registryGoProxy:
			var body struct {
				Version string
				Time    time.Time
			}
			err := getJSON(client, "https://proxy.golang.org/"+source.name+"/@latest", &body)
			return toolRelease{strings.TrimSuffix(strings.TrimPrefix(body.Version, "v"), "+incompatible"), body.Time}, err
		case registryPyPI:
			var body struct {
				Info struct {
					Version string `json:"version"`
				} `json:"info"`
				Releases map[string][]struct {
					Uploaded time.Time `json:"upload_time_iso_8601"`
				} `json:"releases"`
			}
			if err := getJSON(client, "https://pypi.org/pypi/"+source.name+"/json", &body); err != nil {
				return toolRelease{}, err
			}
			files := body.Releases[body.Info.Version]
			if len(files) == 0 {
				return toolRelease{}, fmt.Errorf("pypi lists no files for %s %s", source.name, body.Info.Version)
			}
			return toolRelease{body.Info.Version, files[0].Uploaded}, nil
		case registryNPM:
			var body struct {
				DistTags struct {
					Latest string `json:"latest"`
				} `json:"dist-tags"`
				Time map[string]time.Time `json:"time"`
			}
			err := getJSON(client, "https://registry.npmjs.org/"+source.name, &body)
			return toolRelease{body.DistTags.Latest, body.Time[body.DistTags.Latest]}, err
		case registryGitHubReleases:
			var body struct {
				Tag       string    `json:"tag_name"`
				Published time.Time `json:"published_at"`
			}
			err := getJSON(client, "https://api.github.com/repos/"+source.name+"/releases/latest", &body)
			return toolRelease{strings.TrimPrefix(body.Tag, "v"), body.Published}, err
		}
		return toolRelease{}, fmt.Errorf("unknown registry %q", source.registry)
	}
}

func getJSON(client *http.Client, address string, into any) error {
	response, err := client.Get(address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", address, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return fmt.Errorf("%s: %w", address, err)
	}
	return nil
}
