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
	scan := scheduleScan{root: root, known: known, scheduled: map[string]bool{}, periods: map[string]string{}}
	for index, line := range strings.Split(string(data), "\n") {
		scan.line(fmt.Sprintf("purge-schedule: %s:%d", maintenanceSchedule, index+1), line)
	}
	problems, scheduled, periods := scan.problems, scan.scheduled, scan.periods

	for _, name := range subcommands {
		if !scheduled[name] {
			problems = append(problems, fmt.Sprintf(
				"purge-schedule: `maintenance %s` has no line in %s. The command ships no scheduler on purpose, "+
					"so an unscheduled job never runs: a retention sweep becomes a promise nobody keeps, "+
					"a collector becomes storage nobody reclaims",
				name, maintenanceSchedule))
		}
	}
	problems = append(problems, jobPeriodProblems(filepath.Join(root, filepath.FromSlash(maintenanceMain)), periods, known)...)
	sort.Strings(problems)
	return problems
}

type scheduleScan struct {
	root      string
	known     map[string]bool
	scheduled map[string]bool
	periods   map[string]string
	problems  []string
}

func (s *scheduleScan) line(where, line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	if len(fields) != 2 {
		s.problems = append(s.problems, fmt.Sprintf("%s: want `<period> <subcommand>`, got %q", where, line))
		return
	}
	period, name := fields[0], fields[1]
	timer := filepath.Join(s.root, filepath.FromSlash(maintenanceTimers), "skillhub-"+period+"@.timer")
	_, timerErr := os.Stat(timer)
	if timerErr != nil {
		s.problems = append(s.problems, fmt.Sprintf("%s: period %q has no %s/skillhub-%s@.timer, so `maintenance %s` would never fire",
			where, period, maintenanceTimers, period, name))
	}
	if timerErr == nil && s.known[name] && !s.scheduled[name] {
		s.periods[name] = period
	}
	if !s.known[name] {
		s.problems = append(s.problems, fmt.Sprintf("%s: `maintenance %s` is not a subcommand in %s, so its timer would fail every run",
			where, name, maintenanceMain))
	}
	if s.scheduled[name] {
		s.problems = append(s.problems, fmt.Sprintf("%s: `maintenance %s` is scheduled twice", where, name))
	}
	s.scheduled[name] = true
}

const jobPeriodsVar = "jobPeriods"

func jobPeriodProblems(path string, scheduled map[string]string, known map[string]bool) []string {
	declared, err := maintenanceJobPeriods(path)
	if err != nil {
		return []string{fmt.Sprintf("purge-schedule: %v", err)}
	}
	if declared == nil {
		return []string{fmt.Sprintf(
			"purge-schedule: %s declares no %s, so the overdue alert cannot know how often each job should succeed",
			maintenanceMain, jobPeriodsVar)}
	}
	var problems []string
	for _, name := range sortedKeys(scheduled) {
		if declared[name] != scheduled[name] {
			problems = append(problems, fmt.Sprintf(
				"purge-schedule: %s[%q] = %q but %s runs it %s, so its overdue alert would fire at the wrong time",
				jobPeriodsVar, name, declared[name], maintenanceSchedule, scheduled[name]))
		}
	}
	for _, name := range sortedKeys(declared) {
		if !known[name] {
			problems = append(problems, fmt.Sprintf(
				"purge-schedule: %s[%q] is not a subcommand in %s; delete the entry", jobPeriodsVar, name, maintenanceMain))
		}
	}
	return problems
}

func maintenanceJobPeriods(path string) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var declared map[string]string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != jobPeriodsVar || len(spec.Values) != 1 {
			return true
		}
		if literal, ok := spec.Values[0].(*ast.CompositeLit); ok {
			declared = periodsByJob(literal)
			return false
		}
		return true
	})
	return declared, nil
}

func periodsByJob(literal *ast.CompositeLit) map[string]string {
	declared := map[string]string{}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		period, periodOK := stringLiteral(pair.Key)
		jobs, jobsOK := pair.Value.(*ast.CompositeLit)
		if !periodOK || !jobsOK {
			continue
		}
		for _, job := range jobs.Elts {
			if name, ok := stringLiteral(job); ok {
				declared[name] = period
			}
		}
	}
	return declared
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
