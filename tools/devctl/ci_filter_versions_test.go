package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	filterGroup       = regexp.MustCompile(`^ {12}([a-z0-9-]+):$`)
	filterPattern     = regexp.MustCompile(`^ {14}- '([^']+)'$`)
	ciJobHeader       = regexp.MustCompile(`^ {2}([a-z0-9-]+):$`)
	jobGate           = regexp.MustCompile(`if: needs\.changes\.outputs\.([a-z0-9-]+) == 'true'`)
	versionFileSetter = regexp.MustCompile(`(?:node|go)-version-file: (\S+)`)
)

func changeFilters(workflow string) map[string][]string {
	filters := map[string][]string{}
	group := ""
	for _, line := range strings.Split(workflow, "\n") {
		if match := filterGroup.FindStringSubmatch(line); match != nil {
			group = match[1]
			continue
		}
		if match := filterPattern.FindStringSubmatch(line); match != nil && group != "" {
			filters[group] = append(filters[group], match[1])
		}
	}
	return filters
}

func filterCovers(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if pattern == file {
			return true
		}
		if prefix, ok := strings.CutSuffix(pattern, "**"); ok && strings.HasPrefix(file, prefix) {
			return true
		}
	}
	return false
}

type versionFileGap struct{ job, group, file string }

func versionFilesOutsideTheirFilter(workflow string) []versionFileGap {
	filters := changeFilters(workflow)
	var gaps []versionFileGap
	job, group := "", ""
	for _, line := range strings.Split(workflow, "\n") {
		if match := ciJobHeader.FindStringSubmatch(line); match != nil {
			job, group = match[1], ""
			continue
		}
		if match := jobGate.FindStringSubmatch(line); match != nil {
			group = match[1]
			continue
		}
		if match := versionFileSetter.FindStringSubmatch(line); match != nil && group != "" {
			if !filterCovers(filters[group], match[1]) {
				gaps = append(gaps, versionFileGap{job, group, match[1]})
			}
		}
	}
	return gaps
}

func TestEveryFilteredJobRunsWhenItsToolchainVersionFileChanges(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := strings.ReplaceAll(string(data), "\r\n", "\n")
	if len(changeFilters(workflow)) == 0 {
		t.Fatal("found no change filters in ci.yml")
	}
	for _, gap := range versionFilesOutsideTheirFilter(workflow) {
		t.Errorf("job %s reads %s but the %s filter does not list it, so changing that version runs nothing", gap.job, gap.file, gap.group)
	}
}

func TestAVersionFileIsCoveredOnlyByItsExactPathOrAGlobAboveIt(t *testing.T) {
	t.Parallel()
	const workflow = "jobs:\n" +
		"  changes:\n" +
		"    steps:\n" +
		"      - with:\n" +
		"          filters: |\n" +
		"            web:\n" +
		"              - 'apps/web/**'\n" +
		"            go:\n" +
		"              - 'apps/platform/**'\n" +
		"  web:\n" +
		"    if: needs.changes.outputs.web == 'true'\n" +
		"    steps:\n" +
		"      - with:\n" +
		"          node-version-file: .node-version\n" +
		"  platform:\n" +
		"    if: needs.changes.outputs.go == 'true'\n" +
		"    steps:\n" +
		"      - with:\n" +
		"          go-version-file: apps/platform/go.mod\n" +
		"  audit:\n" +
		"    if: github.event_name == 'push'\n" +
		"    steps:\n" +
		"      - with:\n" +
		"          node-version-file: .node-version\n"
	gaps := versionFilesOutsideTheirFilter(workflow)
	if len(gaps) != 1 || gaps[0] != (versionFileGap{"web", "web", ".node-version"}) {
		t.Fatalf("gaps = %v, want only the web job's .node-version", gaps)
	}
}
