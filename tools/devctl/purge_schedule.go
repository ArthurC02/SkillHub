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
	maintenanceMain     = "apps/platform/cmd/maintenance/main.go"
	maintenanceSchedule = "infra/deploy/control-plane/maintenance-schedule"
	maintenanceTimers   = "infra/deploy/control-plane/systemd"
	purgeSchedFloor     = 5
)

func purgeScheduleProblems(root string) []string {
	subcommands, err := maintenanceSubcommands(filepath.Join(root, filepath.FromSlash(maintenanceMain)))
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %v", err)}
	}
	if len(subcommands) < purgeSchedFloor {
		return []string{fmt.Sprintf(
			"purge-schedule: only %d subcommands found in %s's switch (%v); there have been "+
				"at least %d since M4, so the switch scan is broken rather than the jobs deleted",
			len(subcommands), maintenanceMain, subcommands, purgeSchedFloor)}
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(maintenanceSchedule)))
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %v", err)}
	}

	known := map[string]bool{}
	for _, name := range subcommands {
		known[name] = true
	}
	var problems []string
	scheduled := map[string]bool{}
	for index, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		where := fmt.Sprintf("purge-schedule: %s:%d", maintenanceSchedule, index+1)
		if len(fields) != 2 {
			problems = append(problems, fmt.Sprintf("%s: want `<period> <subcommand>`, got %q", where, line))
			continue
		}
		period, name := fields[0], fields[1]
		timer := filepath.Join(root, filepath.FromSlash(maintenanceTimers), "skillhub-"+period+"@.timer")
		if _, err := os.Stat(timer); err != nil {
			problems = append(problems, fmt.Sprintf("%s: period %q has no %s/skillhub-%s@.timer, so `maintenance %s` would never fire",
				where, period, maintenanceTimers, period, name))
		}
		if !known[name] {
			problems = append(problems, fmt.Sprintf("%s: `maintenance %s` is not a subcommand in %s, so its timer would fail every run",
				where, name, maintenanceMain))
		}
		if scheduled[name] {
			problems = append(problems, fmt.Sprintf("%s: `maintenance %s` is scheduled twice", where, name))
		}
		scheduled[name] = true
	}

	for _, name := range subcommands {
		if !scheduled[name] {
			problems = append(problems, fmt.Sprintf(
				"purge-schedule: `maintenance %s` has no line in %s. The command ships no scheduler on purpose, "+
					"so an unscheduled job never runs: a retention sweep becomes a promise nobody keeps, "+
					"a collector becomes storage nobody reclaims",
				name, maintenanceSchedule))
		}
	}
	sort.Strings(problems)
	return problems
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
