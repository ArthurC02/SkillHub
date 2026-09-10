package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const sdkVersionDockerfile = "infra/images/runtime-agent-sdk/Dockerfile"

var sdkVersionARG = regexp.MustCompile(`^\s*ARG\s+CLAUDE_AGENT_SDK_VERSION\s*=\s*(\S+)`)

type sdkVersionSite struct {
	file string

	what    string
	pattern *regexp.Regexp
}

var sdkVersionSites = []sdkVersionSite{
	{
		file:    "apps/sandbox/cmd/sandboxd/main.go",
		what:    "the compiled default sandboxd advertises in ProviderCapability, which dispatch freezes into runs.runtime_snapshot as the run's permanent runtime_version, I-05",
		pattern: regexp.MustCompile(`SKILLHUB_SANDBOX_RUNTIME_VERSION"\s*,\s*"([^"]*)"`),
	},
	{
		file:    "apps/sandbox/README.md",
		what:    "the default an operator reads out of the settings table",
		pattern: regexp.MustCompile("`SKILLHUB_SANDBOX_RUNTIME_VERSION`\\s*\\|\\s*`([^`]*)`"),
	},
}

func sdkVersionProblems(root string) []string {
	pinned, pinnedLine, problems := sdkVersionAt(
		root, sdkVersionDockerfile, sdkVersionARG, "ARG CLAUDE_AGENT_SDK_VERSION")
	if len(problems) > 0 {
		return problems
	}

	if strings.ContainsAny(pinned, "^~*>=<") || strings.Contains(pinned, "latest") {
		return []string{fmt.Sprintf(
			"sdk-version: %s:%d pins CLAUDE_AGENT_SDK_VERSION=%s, which is a range rather than an exact "+
				"version; ADR-023 決策 1 bars ^, ~ and latest from the image build path, and a range cannot "+
				"be what the copies of it say",
			sdkVersionDockerfile, pinnedLine, pinned)}
	}

	for _, site := range sdkVersionSites {
		value, line, siteProblems := sdkVersionAt(root, site.file, site.pattern, "SKILLHUB_SANDBOX_RUNTIME_VERSION")
		if len(siteProblems) > 0 {
			problems = append(problems, siteProblems...)
			continue
		}
		if value == pinned {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"sdk-version: %s:%d pins the Agent SDK at %s but %s:%d says %s (%s). ADR-023 決策 1 makes the "+
				"image the source of truth and the copies follow it, so bump them in the same commit — and "+
				"if this is an upgrade, ADR-023 §2's four measurements and the %s row are due with it",
			sdkVersionDockerfile, pinnedLine, pinned, site.file, line, value, site.what,
			"infra/images/runtime-agent-sdk/UPGRADES.md"))
	}
	return problems
}

func sdkVersionAt(root, relative string, pattern *regexp.Regexp, subject string) (string, int, []string) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", 0, []string{fmt.Sprintf("sdk-version: cannot read %s: %v", relative, err)}
	}
	var values []string
	var lines []int
	for i, line := range strings.Split(string(data), "\n") {
		if m := pattern.FindStringSubmatch(line); m != nil {
			values = append(values, m[1])
			lines = append(lines, i+1)
		}
	}
	switch len(values) {
	case 1:
		return values[0], lines[0], nil
	case 0:
		return "", 0, []string{fmt.Sprintf(
			"sdk-version: %s no longer states %s in a shape this check recognises; the Agent SDK version is "+
				"still written there and nothing is comparing it with the pin any more. This check has lost "+
				"part of its subject", relative, subject)}
	default:
		return "", 0, []string{fmt.Sprintf(
			"sdk-version: %s states %s %d times (lines %v); the pinned SDK version cannot have two authors "+
				"in one file", relative, subject, len(values), lines)}
	}
}
