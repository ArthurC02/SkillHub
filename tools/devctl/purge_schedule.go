package main

// Every retention sweep cmd/maintenance can run must have a cron line in the
// release checklist's deployment section.
//
// cmd/maintenance ships no scheduler — its own header says so, deliberately:
// 「程式刻意不自帶 scheduler」. That is a reasonable decision and it has one
// consequence, which is that the sweep exists in this repository and runs
// nowhere until an operator wires it. A sweep nobody wired is not a slow sweep;
// it is a retention promise that will never be kept, and the only place that
// difference is visible is a checklist item.
//
// The checklist has such lines for two of the seven subcommands
// (`purge-accounts`, `rotate-partitions`). The other five — analytics, audit,
// run artifacts, datasets, deleted skills — carry the promises made in
// gate-test/consent-and-data-policy.md to people who will sign it, and there is
// nothing between the promise and the operator.
//
// Anchored to the subcommand names because that is what an operator types, and
// read from `main.go`'s switch rather than from its usage string: 02:PORT-004's
// lesson is that a check satisfied by prose is satisfied by prose that lies. A
// `case "purge-x":` added to the dispatch switch joins this comparison by
// existing.

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
	// The deployment section only. A subcommand named in §1's "what the code
	// already does" or in §5's answers is not a subcommand anybody scheduled,
	// and counting those would make this check green by reading its own subject
	// matter back to itself.
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

// deploymentSection returns the body of the checklist's 部署期 chapter — the
// `## 2.` heading through the next `## `.
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

// maintenanceSubcommands reads the case values of the dispatch switch out of the
// AST. Same reason as everywhere else in this package: a text scan finds the
// names in the usage string and the file header, both of which survive deleting
// the code that runs them.
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
