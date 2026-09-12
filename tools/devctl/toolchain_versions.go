package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type versionSite struct {
	file    string
	version *regexp.Regexp
}

type versionGroup struct {
	tool  string
	sites []versionSite
}

var (
	goDirective     = regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)`)
	golangBaseImage = regexp.MustCompile(`(?m)^FROM golang:(\d+\.\d+\.\d+)-`)
	pythonBaseImage = regexp.MustCompile(`(?m)^FROM python:(\d+\.\d+)[.-]`)
	pythonFloor     = regexp.MustCompile(`(?m)^requires-python = ">=(\d+\.\d+)"`)
	uvArg           = regexp.MustCompile(`(?m)^ARG UV_VERSION=(\S+)`)
)

func toolchainPin(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^  ` + key + `: "([^"]+)"`)
}

var versionsThatMoveTogether = []versionGroup{
	{"node", []versionSite{
		{".node-version", regexp.MustCompile(`^(\d+\.\d+\.\d+)\s*$`)},
		{"infra/images/web/Dockerfile", regexp.MustCompile(`(?m)^FROM node:(\d+\.\d+\.\d+)-`)},
		{"infra/images/devtools/Dockerfile", regexp.MustCompile(`(?m)^ARG NODE_VERSION=(\S+)`)},
	}},
	{"go", []versionSite{
		{"apps/platform/go.mod", goDirective},
		{"apps/sandbox/go.mod", goDirective},
		{"tools/devctl/go.mod", goDirective},
		{"tools/codegen/go/go.mod", goDirective},
		{"infra/images/devtools/Dockerfile", golangBaseImage},
		{"infra/images/platform/Dockerfile", golangBaseImage},
		{"tools/codegen/go/Dockerfile", golangBaseImage},
	}},
	{"python", []versionSite{
		{"apps/llm/.python-version", regexp.MustCompile(`^(\d+\.\d+)\s*$`)},
		{"apps/llm/pyproject.toml", pythonFloor},
		{"packages/api-stub-py/pyproject.toml", pythonFloor},
		{"tools/codegen/python/pyproject.toml", regexp.MustCompile(`(?m)^requires-python = "==(\d+\.\d+)\.\*"`)},
		{"infra/images/llm/Dockerfile", pythonBaseImage},
		{"tools/codegen/python/Dockerfile", pythonBaseImage},
		{"infra/images/devtools/Dockerfile", regexp.MustCompile(`uv python install (\d+\.\d+)`)},
	}},
	{"uv", []versionSite{
		{"tools/toolchain.yaml", toolchainPin("uv")},
		{"infra/images/llm/Dockerfile", uvArg},
		{"infra/images/devtools/Dockerfile", uvArg},
		{"tools/codegen/python/Dockerfile", regexp.MustCompile(`astral-sh/uv:([^@\s]+)@`)},
	}},
	{"task", []versionSite{
		{"tools/toolchain.yaml", toolchainPin("task")},
		{"infra/images/devtools/Dockerfile", regexp.MustCompile(`(?m)^ARG TASK_VERSION=(\S+)`)},
	}},
	{"golangci-lint", []versionSite{
		{"tools/toolchain.yaml", toolchainPin("golangci_lint")},
		{"infra/images/devtools/Dockerfile", regexp.MustCompile(`(?m)^ARG GOLANGCI_LINT_VERSION=(\S+)`)},
	}},
	{"syft", []versionSite{
		{".github/workflows/runtime-image.yml", scannerPin("SYFT", "syft")},
		{".github/workflows/image-scan.yml", scannerPin("SYFT", "syft")},
	}},
	{"grype", []versionSite{
		{".github/workflows/runtime-image.yml", scannerPin("GRYPE", "grype")},
		{".github/workflows/image-scan.yml", scannerPin("GRYPE", "grype")},
	}},
}

func scannerPin(key, image string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^  ` + key + `: docker\.io/anchore/` + image + `:(\S+)$`)
}

func versionAgreementProblems(groups []versionGroup, read func(string) (string, error)) []string {
	var problems []string
	for _, group := range groups {
		var found []string
		distinct := map[string]bool{}
		for _, site := range group.sites {
			content, err := read(site.file)
			matches := site.version.FindAllStringSubmatch(content, -1)
			if err != nil || len(matches) == 0 {
				problems = append(problems, fmt.Sprintf("%s: no %s version found where one is expected", site.file, group.tool))
				continue
			}
			for _, match := range matches {
				distinct[match[1]] = true
				found = append(found, site.file+"="+match[1])
			}
		}
		if len(distinct) > 1 {
			sort.Strings(found)
			problems = append(problems, fmt.Sprintf("%s versions disagree (%s); upgrade every site together", group.tool, strings.Join(found, ", ")))
		}
	}
	return problems
}
