package main

import (
	"errors"
	"regexp"
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
		{"every site agrees", "26.10.0\n", "FROM node:26.10.0-slim AS build\nFROM node:26.10.0-slim\n", ""},
		{"one site moved alone", "26.10.0\n", "FROM node:24.21.0-slim\n", "node versions disagree (.node-version=26.10.0, Dockerfile=24.21.0)"},
		{"two stages of one file disagree", "26.10.0\n", "FROM node:26.10.0-slim AS build\nFROM node:26.9.0-slim\n", "Dockerfile=26.10.0, Dockerfile=26.9.0"},
		{"a site lost its version", "26.10.0\n", "FROM node:lts-slim\n", "Dockerfile: no node version found"},
		{"a site file is missing", "", "FROM node:26.10.0-slim\n", ".node-version: no node version found"},
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
	for _, tool := range []string{"node", "node-types", "go", "python", "uv", "task", "golangci-lint", "syft", "grype"} {
		if !tools[tool] {
			t.Errorf("%s is no longer checked for version agreement", tool)
		}
	}
}

func TestNodeTypesMajorMustMatchNode(t *testing.T) {
	t.Parallel()
	var group versionGroup
	for _, candidate := range versionsThatMoveTogether {
		if candidate.tool == "node-types" {
			group = candidate
			break
		}
	}
	if len(group.sites) != 2 {
		t.Fatalf("got %d node-types sites, want runtime and package", len(group.sites))
	}
	for _, c := range []struct {
		name, typesVersion, want string
	}{
		{"same major", "26.6.3", ""},
		{"stale types major", "24.13.4", "node-types versions disagree (.node-version=26, apps/web/package.json=24)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			content := map[string]string{
				".node-version":         "26.10.0\n",
				"apps/web/package.json": `{"devDependencies":{"@types/node":"^` + c.typesVersion + `"}}`,
			}
			problems := versionAgreementProblems([]versionGroup{group}, func(file string) (string, error) {
				return content[file], nil
			})
			if c.want == "" && len(problems) != 0 {
				t.Fatalf("got %v, want none", problems)
			}
			if c.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], c.want)) {
				t.Fatalf("got %v, want one problem containing %q", problems, c.want)
			}
		})
	}
}
