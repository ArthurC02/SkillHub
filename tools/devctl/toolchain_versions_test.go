package main

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestToolchainVersionsMustAgreeAtEverySite(t *testing.T) {
	t.Parallel()
	groups := []versionGroup{{"node", []versionSite{
		{".node-version", regexp.MustCompile(`^(\d+\.\d+\.\d+)\s*$`)},
		{"Dockerfile", regexp.MustCompile(`(?m)^FROM node:(\d+\.\d+\.\d+)-`)},
	}}}
	cases := []struct {
		name, nodeVersion, dockerfile, want string
	}{
		{"every site agrees", "24.21.0\n", "FROM node:24.21.0-slim AS build\nFROM node:24.21.0-slim\n", ""},
		{"one site moved alone", "24.21.0\n", "FROM node:26.0.0-slim\n", "node versions disagree (.node-version=24.21.0, Dockerfile=26.0.0)"},
		{"two stages of one file disagree", "24.21.0\n", "FROM node:24.21.0-slim AS build\nFROM node:24.20.0-slim\n", "Dockerfile=24.20.0, Dockerfile=24.21.0"},
		{"a site lost its version", "24.21.0\n", "FROM node:lts-slim\n", "Dockerfile: no node version found"},
		{"a site file is missing", "", "FROM node:24.21.0-slim\n", ".node-version: no node version found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			read := func(file string) (string, error) {
				content := map[string]string{".node-version": c.nodeVersion, "Dockerfile": c.dockerfile}[file]
				if content == "" {
					return "", errors.New("missing")
				}
				return content, nil
			}
			problems := versionAgreementProblems(groups, read)
			if c.want == "" {
				if len(problems) != 0 {
					t.Fatalf("got %v, want none", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], c.want) {
				t.Fatalf("got %v, want one problem containing %q", problems, c.want)
			}
		})
	}
}

func TestEveryToolThatMovesTogetherHasMoreThanOneSite(t *testing.T) {
	t.Parallel()
	tools := map[string]bool{}
	for _, group := range versionsThatMoveTogether {
		tools[group.tool] = true
		if len(group.sites) < 2 {
			t.Errorf("%s has %d site; one site cannot drift from anything", group.tool, len(group.sites))
		}
	}
	for _, tool := range []string{"node", "go", "python", "uv", "task", "golangci-lint", "docker", "docker-compose", "openapi-ts", "redocly", "ruff", "syft", "grype"} {
		if !tools[tool] {
			t.Errorf("%s is no longer checked for version agreement", tool)
		}
	}
}

func TestUVVersionSitesReadImageStagesAndArguments(t *testing.T) {
	t.Parallel()
	groups := slices.Clone(versionsThatMoveTogether)
	groups = slices.DeleteFunc(groups, func(group versionGroup) bool {
		return group.tool != "uv"
	})
	if len(groups) != 1 {
		t.Fatalf("got %d uv groups, want one", len(groups))
	}
	digest := "@sha256:" + strings.Repeat("a", 64)
	image := "FROM ghcr.io/astral-sh/uv:0.12.3" + digest + " AS uv\n"
	cases := []struct {
		name, path, content, want string
	}{
		{"all installation sites agree", "", "", ""},
		{"the LLM image cannot move alone", "infra/images/llm/Dockerfile", strings.ReplaceAll(image, "0.12.3", "0.12.4"), "infra/images/llm/Dockerfile=0.12.4"},
		{"the codegen image cannot move alone", "tools/codegen/python/Dockerfile", strings.ReplaceAll(image, "0.12.3", "0.12.4"), "tools/codegen/python/Dockerfile=0.12.4"},
		{"the devtools archive cannot move alone", devtoolsDockerfile, "ARG UV_VERSION=0.12.4\n", devtoolsDockerfile + "=0.12.4"},
		{"the manifest cannot move alone", toolchainYAML, "tools:\n  uv: \"0.12.4\"\n", toolchainYAML + "=0.12.4"},
		{"an obsolete LLM argument is not an image stage", "infra/images/llm/Dockerfile", "ARG UV_VERSION=0.12.3\n", "infra/images/llm/Dockerfile: no uv version found"},
		{"a comment is not an LLM image stage", "infra/images/llm/Dockerfile", "# " + image, "infra/images/llm/Dockerfile: no uv version found"},
		{"a comment is not a codegen image stage", "tools/codegen/python/Dockerfile", "# " + image, "tools/codegen/python/Dockerfile: no uv version found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{
				toolchainYAML:                     "tools:\n  uv: \"0.12.3\"\n",
				"infra/images/llm/Dockerfile":     image,
				devtoolsDockerfile:                "ARG UV_VERSION=0.12.3\n",
				"tools/codegen/python/Dockerfile": image,
			}
			if c.path != "" {
				files[c.path] = c.content
			}
			read := func(path string) (string, error) {
				value, ok := files[path]
				if !ok {
					return "", errors.New("missing")
				}
				return value, nil
			}
			problems := versionAgreementProblems(groups, read)
			if c.want == "" {
				if len(problems) != 0 {
					t.Fatalf("got %v, want no version disagreement", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], c.want) {
				t.Fatalf("got %v, want one problem containing %q", problems, c.want)
			}
		})
	}
}
