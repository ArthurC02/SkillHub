package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	maintenanceMain  = "apps/platform/cmd/maintenance/main.go"
	deploymentDoc    = "docs/plans/mvp/m4/release-checklist.md"
	purgeSchedFloor  = 5
	purgeSchedPrefix = "purge-"
	rotateSubcommand = "rotate-partitions"
)

func purgeScheduleProblems(root string) []string {
	subcommands, err := maintenanceSubcommands(filepath.Join(root, filepath.FromSlash(maintenanceMain)))
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %v", err)}
	}
	var scheduled []string
	for _, name := range subcommands {
		if strings.HasPrefix(name, purgeSchedPrefix) || name == rotateSubcommand {
			scheduled = append(scheduled, name)
		}
	}
	if len(scheduled) < purgeSchedFloor {
		return []string{fmt.Sprintf(
			"purge-schedule: only %d of %s's subcommands look like retention sweeps (%v); there have been "+
				"at least %d since M4, so the switch scan is broken rather than the sweeps deleted",
			len(scheduled), maintenanceMain, scheduled, purgeSchedFloor)}
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(deploymentDoc)))
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %v", err)}
	}

	section, err := deploymentSection(string(data))
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %s: %v", deploymentDoc, err)}
	}

	var problems []string
	for _, name := range scheduled {
		var scheduledHere bool
		for _, line := range strings.Split(section, "\n") {
			if strings.Contains(line, name) && strings.Contains(line, "cron") {
				scheduledHere = true
				break
			}
		}
		if !scheduledHere {
			problems = append(problems, fmt.Sprintf(
				"purge-schedule: `maintenance %s` is a retention sweep with no cron line in %s's deployment "+
					"section (§2). The command ships no scheduler on purpose, so an unscheduled sweep is a "+
					"retention promise nothing will keep — and the promises are in "+
					"gate-test/consent-and-data-policy.md, signed by people",
				name, deploymentDoc))
		}
	}
	sort.Strings(problems)
	return problems
}

func deploymentSection(text string) (string, error) {
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## 2.") {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("no `## 2.` deployment chapter; this check has lost half its subject")
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			return strings.Join(lines[start:i], "\n"), nil
		}
	}
	return strings.Join(lines[start:], "\n"), nil
}

func maintenanceSubcommands(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		clause, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range clause.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if value, err := strconv.Unquote(lit.Value); err == nil {
				names = append(names, value)
			}
		}
		return true
	})
	sort.Strings(names)
	return names, nil
}
