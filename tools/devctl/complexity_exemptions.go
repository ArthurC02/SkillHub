package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const gocognitComplexityCeiling = 30

type complexityGateModule struct {
	lintPath         string
	registeredExempt int
}

var complexityGateModules = []complexityGateModule{
	{lintPath: "apps/platform/.golangci.yml", registeredExempt: 12},
	{lintPath: "apps/sandbox/.golangci.yml", registeredExempt: 2},
	{lintPath: "tools/devctl/.golangci.yml", registeredExempt: 15},
}

var (
	gocognitEnabled       = regexp.MustCompile(`(?m)^\s*-\s*gocognit\s*$`)
	gocognitMinComplexity = regexp.MustCompile(`(?m)^\s*min-complexity:\s*(\d+)\s*$`)
	gocognitFunctionRule  = regexp.MustCompile(`(?m)^\s*text:\s*'func `)
)

func complexityExemptionProblems(root string) []string {
	var problems []string
	for _, module := range complexityGateModules {
		problems = append(problems, complexityGateModuleProblems(root, module)...)
	}
	return problems
}

func complexityGateModuleProblems(root string, module complexityGateModule) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(module.lintPath)))
	if err != nil {
		return []string{fmt.Sprintf("complexity-exemptions: %s: %v", module.lintPath, err)}
	}
	text := stripYAMLComments(string(data))

	if !gocognitEnabled.MatchString(text) {
		return []string{fmt.Sprintf(
			"complexity-exemptions: %s does not enable gocognit under linters.enable", module.lintPath)}
	}

	var problems []string
	if m := gocognitMinComplexity.FindStringSubmatch(text); m == nil {
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s sets no gocognit min-complexity", module.lintPath))
	} else if threshold, _ := strconv.Atoi(m[1]); threshold > gocognitComplexityCeiling {
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s sets min-complexity %d, above the registered ceiling %d; "+
				"raising the gate hides functions instead of splitting them",
			module.lintPath, threshold, gocognitComplexityCeiling))
	}

	actual := len(gocognitFunctionRule.FindAllString(text, -1))
	switch {
	case actual > module.registeredExempt:
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s carries %d per-function gocognit exemptions, more than the registered %d; "+
				"exemptions only shrink, split the new function instead of adding another one",
			module.lintPath, actual, module.registeredExempt))
	case actual < module.registeredExempt:
		problems = append(problems, fmt.Sprintf(
			"complexity-exemptions: %s carries %d per-function gocognit exemptions, fewer than the registered %d; "+
				"lower complexityGateModules in tools/devctl/complexity_exemptions.go to match",
			module.lintPath, actual, module.registeredExempt))
	}
	return problems
}
