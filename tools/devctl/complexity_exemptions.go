package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const gocognitComplexityCeiling = 30

var complexityGateLintPaths = []string{
	"apps/platform/.golangci.yml",
	"apps/sandbox/.golangci.yml",
	"tools/devctl/.golangci.yml",
}

var (
	gocognitEnabled       = regexp.MustCompile(`(?m)^\s*-\s*gocognit\s*$`)
	gocognitMinComplexity = regexp.MustCompile(`(?m)^\s*min-complexity:\s*(\d+)\s*$`)
	gocognitFunctionRule  = regexp.MustCompile(`(?m)^\s*text:\s*'func `)
)

func complexityExemptionProblems(root string) []string {
	var problems []string
	for _, lintPath := range complexityGateLintPaths {
		problems = append(problems, complexityGateProblems(root, lintPath)...)
	}
	return problems
}

func complexityGateProblems(root, lintPath string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lintPath)))
	if err != nil {
		return []string{fmt.Sprintf("complexity-exemptions: %s: %v", lintPath, err)}
	}
	text := stripYAMLComments(string(data))

	if !gocognitEnabled.MatchString(text) {
		return []string{fmt.Sprintf(
			"complexity-exemptions: %s does not enable gocognit under linters.enable", lintPath)}
	}

	var problems []string
	if m := gocognitMinComplexity.FindStringSubmatch(text); m == nil {
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s sets no gocognit min-complexity", lintPath))
	} else if threshold, _ := strconv.Atoi(m[1]); threshold > gocognitComplexityCeiling {
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s sets min-complexity %d, above the registered ceiling %d; "+
				"raising the gate hides functions instead of splitting them",
			lintPath, threshold, gocognitComplexityCeiling))
	}

	if exempt := len(gocognitFunctionRule.FindAllString(text, -1)); exempt > 0 {
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s exempts %d functions by name; none are allowed, "+
				"split the function instead of excusing it",
			lintPath, exempt))
	}
	return problems
}
