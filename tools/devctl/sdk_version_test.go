package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tree in the shape of the real one: the Dockerfile that pins, and the two
// files that copy the pin.
func writeSDKVersion(t *testing.T, dockerfile, mainGo, readme string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range map[string]string{
		sdkVersionDockerfile:                dockerfile,
		"apps/sandbox/cmd/sandboxd/main.go": mainGo,
		"apps/sandbox/README.md":            readme,
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The real lines, trimmed, plus the two `${CLAUDE_AGENT_SDK_VERSION}`
// interpolations further down the real Dockerfile. Those are here on purpose:
// they are the reason sdkVersionARG is anchored to the ARG line rather than to
// the name, and a version that was not would call this file three authors of
// one value.
func sdkDockerfile(version string) string {
	return "FROM node:22-bookworm-slim@sha256:d649c27d\n" +
		"# The SDK version is pinned, not floated (I-05).\n" +
		"ARG CLAUDE_AGENT_SDK_VERSION=" + version + "\n" +
		"ARG IMAGE_VERSION=2026.08-3\n" +
		"RUN npm install -g \"@anthropic-ai/claude-agent-sdk@${CLAUDE_AGENT_SDK_VERSION}\"\n" +
		"ENV SKILLHUB_RUNTIME_VERSION=${CLAUDE_AGENT_SDK_VERSION}\n"
}

func sdkMainGo(version string) string {
	return "package main\n\n" +
		"// SKILLHUB_SANDBOX_RUNTIME_VERSION is read below.\n" +
		"var _ = sandbox.RuntimeCapability{\n" +
		"\tVersions: []string{envOr(\"SKILLHUB_SANDBOX_RUNTIME_VERSION\", \"" + version + "\")},\n" +
		"}\n"
}

func sdkReadme(version string) string {
	return "| 環境變數 | 預設 | 說明 |\n| --- | --- | --- |\n" +
		"| `SKILLHUB_SANDBOX_IMAGE` | `skillhub/runtime-agent-sdk:2026.08-3` | Runtime Image |\n" +
		"| `SKILLHUB_SANDBOX_RUNTIME_VERSION` | `" + version + "` | 宣告在 Capability 的 Agent SDK 版本，須與 Image 內一致 |\n"
}

func TestSDKVersionAcceptsCopiesThatAgree(t *testing.T) {
	t.Parallel()
	root := writeSDKVersion(t, sdkDockerfile("0.3.233"), sdkMainGo("0.3.233"), sdkReadme("0.3.233"))
	if problems := sdkVersionProblems(root); len(problems) != 0 {
		t.Fatalf("three copies of 0.3.233 were rejected: %v", problems)
	}
}

// The defect this exists for: the ARG is bumped and the Go default is not, so
// every Run afterwards is stamped with a version that never ran.
func TestSDKVersionCatchesAGoDefaultLeftBehind(t *testing.T) {
	t.Parallel()
	root := writeSDKVersion(t, sdkDockerfile("0.4.1"), sdkMainGo("0.3.233"), sdkReadme("0.4.1"))
	problems := sdkVersionProblems(root)
	if len(problems) != 1 {
		t.Fatalf("expected exactly one problem, got %#v", problems)
	}
	// Both sites and both values, or the message cannot be acted on.
	for _, needle := range []string{
		sdkVersionDockerfile + ":3", "0.4.1",
		"apps/sandbox/cmd/sandboxd/main.go:5", "0.3.233",
		"runs.runtime_snapshot",
	} {
		if !strings.Contains(problems[0], needle) {
			t.Fatalf("problem does not name %q: %q", needle, problems[0])
		}
	}
}

func TestSDKVersionCatchesADocLeftBehind(t *testing.T) {
	t.Parallel()
	root := writeSDKVersion(t, sdkDockerfile("0.4.1"), sdkMainGo("0.4.1"), sdkReadme("0.3.233"))
	problems := sdkVersionProblems(root)
	if len(problems) != 1 {
		t.Fatalf("expected exactly one problem, got %#v", problems)
	}
	if !strings.Contains(problems[0], "apps/sandbox/README.md:4") || !strings.Contains(problems[0], "0.3.233") {
		t.Fatalf("problem does not name the README site and its value: %q", problems[0])
	}
}

// Losing a subject is a failure, not a pass. Each of these used to be the way
// checks in this repository died quietly.
func TestSDKVersionRefusesToCompareNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                       string
		dockerfile, mainGo, readme string
		wantProblems               int
		needle                     string
	}{
		{
			name:       "the pin is gone",
			dockerfile: "FROM node:22-bookworm-slim@sha256:d649c27d\nRUN npm install -g @anthropic-ai/claude-agent-sdk\n",
			mainGo:     sdkMainGo("0.3.233"), readme: sdkReadme("0.3.233"),
			wantProblems: 1, needle: "lost part of its subject",
		},
		{
			name:       "the pin has two authors",
			dockerfile: sdkDockerfile("0.3.233") + "ARG CLAUDE_AGENT_SDK_VERSION=0.4.1\n",
			mainGo:     sdkMainGo("0.3.233"), readme: sdkReadme("0.3.233"),
			wantProblems: 1, needle: "two authors in one file",
		},
		{
			name:       "the pin became a range",
			dockerfile: sdkDockerfile("^0.3.233"),
			mainGo:     sdkMainGo("^0.3.233"), readme: sdkReadme("^0.3.233"),
			wantProblems: 1, needle: "ADR-023 決策 1 bars",
		},
		{
			name:         "the Go default was reworded",
			dockerfile:   sdkDockerfile("0.3.233"),
			mainGo:       "package main\n\nvar version = runtimeVersionFromSomewhereElse()\n",
			readme:       sdkReadme("0.3.233"),
			wantProblems: 1, needle: "apps/sandbox/cmd/sandboxd/main.go no longer states",
		},
		{
			name:       "both copies were reworded",
			dockerfile: sdkDockerfile("0.3.233"),
			mainGo:     "package main\n", readme: "| 環境變數 | 預設 |\n",
			wantProblems: 2, needle: "no longer states",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := sdkVersionProblems(writeSDKVersion(t, tc.dockerfile, tc.mainGo, tc.readme))
			if len(problems) != tc.wantProblems {
				t.Fatalf("expected %d problem(s), got %#v", tc.wantProblems, problems)
			}
			if !strings.Contains(strings.Join(problems, " "), tc.needle) {
				t.Fatalf("no problem mentions %q: %#v", tc.needle, problems)
			}
		})
	}
}

// The tree as it actually stands. Without this the fixtures above only prove
// the patterns match strings written to match them.
func TestSDKVersionAcceptsTheRealTree(t *testing.T) {
	t.Parallel()
	// ../.. is the repo root, the same anchor retention_floor_test.go uses.
	if problems := sdkVersionProblems("../.."); len(problems) != 0 {
		t.Fatalf("the repository's own copies of the Agent SDK version disagree: %v", problems)
	}
}
