package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const envExampleDoc = ".env.example"

var (
	sqlQueryHeader = regexp.MustCompile(`^--\s*name:\s*(\S+)`)

	sqlRetentionLiteral = regexp.MustCompile(`now\(\)\s*\+\s*interval\s*'\s*(\d+)\s*([a-z]+?)s?\s*'`)

	sqlRetentionParam = regexp.MustCompile(`now\(\)\s*\+\s*(@\w+|\$\d+|sqlc\.arg\([^)]*\))`)
)

const analyticsFunnelFloor = 180 * 24 * time.Hour

const observationWindowDoc = "docs/plans/mvp/gate-test/consent-and-data-policy.md"

var observationWindowHeading = regexp.MustCompile(`(?m)^#{2,4}\s.*封閉測試（(\d+)\s*天`)

func retentionFloorProblems(root string) []string {
	var problems []string

	sqlProblems, artifact, artifactWhere := runArtifactRetention(root)
	problems = append(problems, sqlProblems...)
	traceProblems, trace := envRetention(root, "TRACE_RETENTION")
	problems = append(problems, traceProblems...)
	if len(sqlProblems) == 0 && len(traceProblems) == 0 && artifact < trace {
		problems = append(problems, fmt.Sprintf(
			"retention-floor: 02:NFR-002a rule 2 requires Run Artifact retention >= the re-evaluation "+
				"window (TRACE_RETENTION), but %s stamps %s while %s states TRACE_RETENTION=%s — %s "+
				"short. A re-evaluation inside that gap reads an EMPTY artifact manifest and the judge "+
				"decides on it, so the wrong thing is not a sentence in a report, it is the input to an "+
				"append-only verdict (04 丙-13). Raise the constant or lower TRACE_RETENTION",
			artifactWhere, artifact, envExampleDoc, trace, trace-artifact))
	}

	windowProblems, window := observationWindow(root)
	problems = append(problems, windowProblems...)
	downloadProblems, download := envRetention(root, "DOWNLOAD_ARTIFACT_RETENTION")
	problems = append(problems, downloadProblems...)
	if len(windowProblems) == 0 && len(downloadProblems) == 0 && download < window {
		problems = append(problems, fmt.Sprintf(
			"retention-floor: 02:NFR-002a rule 1 requires download retention >= the current observation "+
				"window, but %s states DOWNLOAD_ARTIFACT_RETENTION=%s while %s runs for %s — %s short. "+
				"A participant who comes back inside the study and finds their package gone makes 01 "+
				"§11.2's last funnel segment measure the expiry instead of the behaviour; this exact "+
				"value was set to 7 days and corrected the same day for that reason",
			envExampleDoc, download, observationWindowDoc, window, window-download))
	}

	analyticsProblems, analytics := envRetention(root, "ANALYTICS_RETENTION")
	problems = append(problems, analyticsProblems...)
	if len(analyticsProblems) == 0 && analytics < analyticsFunnelFloor {
		problems = append(problems, fmt.Sprintf(
			"retention-floor: 02:NFR-002a rule 3 requires analytics retention >= one complete funnel, "+
				"and the floor pinned in tools/devctl/retention_floor.go is %s. "+
				"%s states ANALYTICS_RETENTION=%s. The funnel's last segment is "+
				"「首次使用後再回來」 and it crosses months, so a shorter window collects events that "+
				"can never be joined into an answer",
			analyticsFunnelFloor, envExampleDoc, analytics))
	}

	sort.Strings(problems)
	return problems
}

func observationWindow(root string) ([]string, time.Duration) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(observationWindowDoc)))
	if err != nil {
		return []string{fmt.Sprintf("retention-floor: %v", err)}, 0
	}
	matches := observationWindowHeading.FindAllStringSubmatch(string(data), -1)
	switch len(matches) {
	case 1:
	case 0:
		return []string{fmt.Sprintf(
			"retention-floor: %s no longer has a 封閉測試（N 天） section heading; it is the only stated "+
				"length of the observation window 02:NFR-002a rule 1 measures against, so this check has "+
				"lost half its subject", observationWindowDoc)}, 0
	default:
		return []string{fmt.Sprintf(
			"retention-floor: %s declares the closed-beta length %d times; the observation window cannot "+
				"have two lengths", observationWindowDoc, len(matches))}, 0
	}
	days, err := strconv.Atoi(matches[0][1])
	if err != nil || days <= 0 {
		return []string{fmt.Sprintf(
			"retention-floor: %s states a closed-beta length of %q days, which is not a positive number",
			observationWindowDoc, matches[0][1])}, 0
	}
	return nil, time.Duration(days) * 24 * time.Hour
}

const (
	runArtifactRetentionPackage = "apps/platform/internal/trial/execution"
	runArtifactRetentionName    = "runArtifactRetention"
)

func runArtifactRetention(root string) (problems []string, retention time.Duration, where string) {
	problems = append(problems, sqlStampedRunArtifactRetention(root)...)

	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(runArtifactRetentionPackage), "*.go"))
	if err != nil {
		return append(problems, fmt.Sprintf("retention-floor: %v", err)), 0, ""
	}
	type site struct {
		where string
		value ast.Expr
	}
	var found []site
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			problems = append(problems, fmt.Sprintf("retention-floor: cannot parse %s: %v", file, parseErr))
			continue
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if name.Name != runArtifactRetentionName || i >= len(value.Values) {
						continue
					}
					relative, relErr := filepath.Rel(root, file)
					if relErr != nil {
						relative = file
					}
					found = append(found, site{
						where: fmt.Sprintf("%s:%d", filepath.ToSlash(relative), fset.Position(name.Pos()).Line),
						value: value.Values[i],
					})
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return problems, 0, ""
	}

	switch len(found) {
	case 1:
	case 0:
		return []string{fmt.Sprintf(
			"retention-floor: no non-test file in %s declares const %s; either run-output retention moved "+
				"somewhere this check cannot see or the manifest no longer stamps one, and either way nothing "+
				"is enforcing 02:NFR-002a rule 2. This check has lost its subject",
			runArtifactRetentionPackage, runArtifactRetentionName)}, 0, ""
	default:
		var sites []string
		for _, f := range found {
			sites = append(sites, f.where)
		}
		sort.Strings(sites)
		return []string{fmt.Sprintf(
			"retention-floor: %d declarations of %s (%s); 02:NFR-002a rule 2 constrains one number and "+
				"there are now two authors of it", len(found), runArtifactRetentionName, strings.Join(sites, ", "))}, 0, ""
	}

	duration, ok := constantDuration(found[0].value)
	if !ok || duration <= 0 {
		return []string{fmt.Sprintf(
			"retention-floor: %s declares %s as something other than a positive product of integer "+
				"literals and time units; 02:NFR-002a rule 2 compares it against TRACE_RETENTION, so this "+
				"check has lost its subject", found[0].where, runArtifactRetentionName)}, 0, ""
	}
	return nil, duration, found[0].where
}

func constantDuration(expr ast.Expr) (time.Duration, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return constantDuration(e.X)
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(e.Value, 0, 64)
		return time.Duration(n), err == nil
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok || pkg.Name != "time" {
			return 0, false
		}
		unit, ok := goDurationUnits[e.Sel.Name]
		return unit, ok
	case *ast.BinaryExpr:
		if e.Op != token.MUL {
			return 0, false
		}
		left, ok := constantDuration(e.X)
		if !ok {
			return 0, false
		}
		right, ok := constantDuration(e.Y)
		return left * right, ok
	}
	return 0, false
}

func sqlStampedRunArtifactRetention(root string) []string {
	files, err := filepath.Glob(filepath.Join(root, "db", "queries", "*.sql"))
	if err != nil {
		return []string{fmt.Sprintf("retention-floor: %v", err)}
	}
	var problems []string
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("retention-floor: cannot read %s: %v", file, readErr))
			continue
		}
		relative, relErr := filepath.Rel(root, file)
		if relErr != nil {
			relative = file
		}
		for _, statement := range sqlStatements(string(data)) {
			code := strings.ToLower(sqlCode(statement.code))
			if strings.Contains(code, "insert into artifacts") && strings.Contains(code, "'run_output'") &&
				(sqlRetentionLiteral.MatchString(code) || sqlRetentionParam.MatchString(code)) {
				problems = append(problems, fmt.Sprintf(
					"retention-floor: %s (%s) stamps run-output retention in SQL while %s declares %s; "+
						"02:NFR-002a rule 2 constrains one number and there are now two authors of it",
					filepath.ToSlash(relative), statement.name, runArtifactRetentionPackage, runArtifactRetentionName))
			}
		}
	}
	return problems
}

func envRetention(root, name string) (problems []string, retention time.Duration) {
	data, err := os.ReadFile(filepath.Join(root, envExampleDoc))
	if err != nil {
		return []string{fmt.Sprintf("retention-floor: cannot read %s: %v", envExampleDoc, err)}, 0
	}
	pattern := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(name) + `\s*=\s*(\S*)`)
	var values []string
	var lines []int
	for i, line := range strings.Split(string(data), "\n") {
		if m := pattern.FindStringSubmatch(line); m != nil {
			values = append(values, m[1])
			lines = append(lines, i+1)
		}
	}
	switch len(values) {
	case 1:
	case 0:
		return []string{fmt.Sprintf(
			"retention-floor: %s no longer assigns %s; it is the only stated value 02:NFR-002a measures "+
				"against, so this check has lost half a floor's subject", envExampleDoc, name)}, 0
	default:
		return []string{fmt.Sprintf(
			"retention-floor: %s assigns %s %d times (lines %v); a retention window cannot have two values",
			envExampleDoc, name, len(values), lines)}, 0
	}

	parsed, err := time.ParseDuration(values[0])
	if err != nil {
		return []string{fmt.Sprintf(
			"retention-floor: %s:%d %s=%q is not a Go duration (%v); cmd/maintenance is fail-closed on "+
				"these, so the sweep that reads it is already refusing to run",
			envExampleDoc, lines[0], name, values[0], err)}, 0
	}
	if parsed <= 0 {
		return []string{fmt.Sprintf(
			"retention-floor: %s:%d %s=%q is not positive", envExampleDoc, lines[0], name, values[0])}, 0
	}
	return nil, parsed
}

type sqlCodeLine struct {
	number int
	text   string
}

type sqlStatement struct {
	name string
	line int
	code []sqlCodeLine
}

func sqlStatements(text string) []sqlStatement {
	var statements []sqlStatement
	for i, line := range strings.Split(text, "\n") {
		if m := sqlQueryHeader.FindStringSubmatch(line); m != nil {
			statements = append(statements, sqlStatement{name: m[1], line: i + 1})
			continue
		}
		if len(statements) == 0 || strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		current := &statements[len(statements)-1]
		current.code = append(current.code, sqlCodeLine{number: i + 1, text: line})
	}
	return statements
}

func sqlCode(lines []sqlCodeLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.text)
		b.WriteByte('\n')
	}
	return b.String()
}
