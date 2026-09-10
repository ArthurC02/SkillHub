package main

import (
	"fmt"
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

var sqlIntervalUnits = map[string]time.Duration{
	"second": time.Second,
	"minute": time.Minute,
	"hour":   time.Hour,
	"day":    24 * time.Hour,
	"week":   7 * 24 * time.Hour,
}

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
				"append-only verdict (04 丙-13). Raise the literal or lower TRACE_RETENTION",
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
				"and the floor pinned in tools/devctl/retention_floor.go is %s (ADR-029 決策 5's own "+
				"proposal). %s states ANALYTICS_RETENTION=%s. The funnel's last segment is "+
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

func runArtifactRetention(root string) (problems []string, retention time.Duration, where string) {
	files, err := filepath.Glob(filepath.Join(root, "db", "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		return []string{
			"retention-floor: db/queries/*.sql matched no files; this check has lost its subject",
		}, 0, ""
	}

	type candidate struct {
		file, name string
		line       int
		code       []sqlCodeLine
	}
	var found []candidate
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
		relative = filepath.ToSlash(relative)
		for _, statement := range sqlStatements(string(data)) {

			code := strings.ToLower(sqlCode(statement.code))
			if !strings.Contains(code, "insert into artifacts") ||
				!strings.Contains(code, "'run_output'") ||
				!strings.Contains(code, "expires_at") {
				continue
			}
			found = append(found, candidate{relative, statement.name, statement.line, statement.code})
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return problems, 0, ""
	}

	switch len(found) {
	case 1:
	case 0:
		return []string{
			"retention-floor: no statement in db/queries/*.sql INSERTs a kind='run_output' row into " +
				"artifacts with an expires_at; either run-output retention moved somewhere this check " +
				"cannot see or the manifest is no longer written, and either way nothing is enforcing " +
				"02:NFR-002a rule 2. This check has lost its subject",
		}, 0, ""
	default:
		var sites []string
		for _, c := range found {
			sites = append(sites, fmt.Sprintf("%s:%d %s", c.file, c.line, c.name))
		}
		sort.Strings(sites)
		return []string{fmt.Sprintf(
			"retention-floor: %d statements stamp a retention on kind='run_output' artifacts (%s); "+
				"02:NFR-002a rule 2 constrains one number and there are now two authors of it",
			len(found), strings.Join(sites, ", "))}, 0, ""
	}

	statement := found[0]
	site := fmt.Sprintf("%s (%s)", statement.file, statement.name)
	for _, line := range statement.code {
		m := sqlRetentionLiteral.FindStringSubmatch(line.text)
		if m == nil {
			continue
		}
		count, _ := strconv.Atoi(m[1])
		unit, ok := sqlIntervalUnits[m[2]]
		if !ok {
			return []string{fmt.Sprintf(
				"retention-floor: %s:%d stamps run-output retention as interval '%s %s', and a %s has no "+
					"fixed length; 02:NFR-002a rule 2 compares it against a Go duration, so write it in "+
					"days or hours",
				statement.file, line.number, m[1], m[2], m[2])}, 0, ""
		}
		return nil, time.Duration(count) * unit, fmt.Sprintf("%s:%d", statement.file, line.number)
	}

	if m := sqlRetentionParam.FindStringSubmatch(sqlCode(statement.code)); m != nil {
		return []string{fmt.Sprintf(
			"retention-floor: %s stamps run-output retention from the parameter %s rather than a literal, "+
				"so the value now lives in a deployment and this repository can no longer compare it with "+
				"TRACE_RETENTION. 02:NFR-002a rule 2 still binds and is now unenforceable from here: give "+
				"the variable a name in %s next to TRACE_RETENTION and point this check at both, or record "+
				"in 05 who compares the pair at deploy time",
			site, m[1], envExampleDoc)}, 0, ""
	}
	return []string{fmt.Sprintf(
		"retention-floor: %s writes expires_at with neither a `now() + interval '…'` literal nor a "+
			"parameter this check recognises; run-output retention has been reworded and 02:NFR-002a "+
			"rule 2 is comparing nothing. This check has lost its subject", site)}, 0, ""
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
