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

// 02:NFR-002a rule 2, which had no machine.
//
// The clause, verbatim: 「**Run Artifact 的保存期限 ≥ 可重評窗。** 可重評窗指
// Trace 的保存期限（評估的證據來源）。…兩者不相等時，見 `EVAL-001` 的過期分
// 支。」 and, closing the section, 「三條都是下界，不是建議值。」 So: a floor,
// not a suggestion, with a named mitigation for the case where the floor is not
// met — the mitigation does not raise the floor.
//
// The two numbers live in different worlds and nothing has ever compared them:
//
//	artifact side  a SQL literal that ships — `now() + interval '30 days'` on the
//	               INSERT that records a run's output manifest
//	trace side     TRACE_RETENTION, a deployment variable with no code default,
//	               whose only stated value is the one in .env.example
//
// WHAT THIS PROVES AND WHAT IT DOES NOT. It proves that the two numbers THIS
// REPOSITORY STATES are consistent with the rule this repository wrote. It does
// not prove that any deployment satisfies 02:NFR-002a rule 2, and it cannot:
// .env.example is a template, not a deployment, so an operator who sets
// TRACE_RETENTION=4000h violates the floor and nothing here will ever see it.
// The only thing that could is an assertion where the two values meet, and they
// never meet — one is compiled into a Postgres statement, the other is read by
// cmd/maintenance out of its own environment. What is checkable from the
// repository is the pair of stated values, and that is exactly what this checks.
//
// The failure mode this is built against is the repo's own recurring one: a
// check that stops finding its subject and passes anyway. The subject has
// already moved once — the query was called RecordRunArtifact when this was
// commissioned and is called InsertRunArtifact now — so nothing here is anchored
// to a query name. It is anchored to what the statement does (an INSERT into
// artifacts of kind 'run_output' carrying expires_at), and zero matches, two
// matches, a reworded literal or a literal that became a deployment parameter
// are each a loud failure rather than a quiet comparison of nothing.

const envExampleDoc = ".env.example"

var (
	// A sqlc statement header: `-- name: InsertRunArtifact :execrows`.
	sqlQueryHeader = regexp.MustCompile(`^--\s*name:\s*(\S+)`)
	// The retention literal. Whitespace is loose because the file's is.
	sqlRetentionLiteral = regexp.MustCompile(`now\(\)\s*\+\s*interval\s*'\s*(\d+)\s*([a-z]+?)s?\s*'`)
	// The shape it takes if it becomes a deployment variable: `now() + @retention`,
	// `now() + $9`, `now() + sqlc.arg(retention)`.
	sqlRetentionParam = regexp.MustCompile(`now\(\)\s*\+\s*(@\w+|\$\d+|sqlc\.arg\([^)]*\))`)
	// `TRACE_RETENTION=2160h`, on its own line, uncommented.
	envTraceRetention = regexp.MustCompile(`^\s*TRACE_RETENTION\s*=\s*(\S*)`)
)

// Postgres interval units with a fixed length. `month` and `year` are missing on
// purpose: they have no fixed number of hours, so a retention written in them
// cannot be compared with a Go duration without picking a lie. Better to say so.
//
// `day` is 24h here. Postgres' `interval '30 days'` is 30 calendar days, which
// differs from 720h across a DST boundary by an hour. An hour does not decide a
// 60-day gap, and pretending otherwise would need a timezone this file has no
// business knowing.
var sqlIntervalUnits = map[string]time.Duration{
	"second": time.Second,
	"minute": time.Minute,
	"hour":   time.Hour,
	"day":    24 * time.Hour,
	"week":   7 * 24 * time.Hour,
}

// The one shortfall this repository has already found, argued about and written
// down, pinned to the exact pair of numbers that produce it.
//
// THIS IS NOT THE RULE BEING SATISFIED. As the tree stands, Run Artifact
// retention (30 days) is SHORTER than the re-evaluation window (TRACE_RETENTION
// 2160h = 90 days), so 02:NFR-002a rule 2 is violated, and days 31-90 of a
// re-evaluation read an empty artifact manifest — the shape 04 丙-13 needed a
// backfill to undo. 03:EVAL-014 is the open work item for it and names both
// exits: implement 02:EVAL-001's expired branch, or raise the SQL literal to 90
// days, at which point that work item disappears.
//
// It is recorded here rather than tolerated silently for the reason
// db/query-owners.yaml gives for its own `allow:` list: 存量漂移清單，不是擴充
// 點. Because both numbers are pinned, the gap cannot grow, cannot move and
// cannot be forgotten — changing either number turns this red, and CLOSING the
// gap turns it red too, because a declared shortfall that no longer exists is
// the same lost-subject failure arriving from the other direction.
//
// When runs.sql says 90 days: set this to nil in the same commit.
var nfr002aKnownShortfall = &retentionShortfall{
	artifact: 30 * 24 * time.Hour,
	trace:    2160 * time.Hour,
	tracked:  "03:EVAL-014",
}

type retentionShortfall struct {
	artifact time.Duration
	trace    time.Duration
	tracked  string
}

func retentionFloorProblems(root string) []string {
	return retentionFloorProblemsFor(root, nfr002aKnownShortfall)
}

// retentionFloorProblemsFor takes the declared shortfall as an argument so a
// test can drive both halves of it — the roster in shared_number.go is checked
// the same way and for the same reason: a constant nothing can vary is a
// constant nothing can test.
func retentionFloorProblemsFor(root string, declared *retentionShortfall) []string {
	problems, artifact, artifactWhere := runArtifactRetention(root)
	traceProblems, trace := traceRetention(root)
	problems = append(problems, traceProblems...)
	if len(problems) > 0 {
		sort.Strings(problems)
		return problems
	}

	if artifact >= trace {
		if declared != nil {
			return []string{fmt.Sprintf(
				"retention-floor: %s stamps %s and %s states TRACE_RETENTION=%s, so 02:NFR-002a rule 2 is "+
					"now MET, but tools/devctl/retention_floor.go still declares the shortfall tracked as "+
					"%s; set nfr002aKnownShortfall to nil in the commit that closed the gap",
				artifactWhere, artifact, envExampleDoc, trace, declared.tracked)}
		}
		return nil
	}

	if declared != nil && declared.artifact == artifact && declared.trace == trace {
		return nil
	}
	pinned := "no shortfall is declared in tools/devctl/retention_floor.go"
	if declared != nil {
		pinned = fmt.Sprintf(
			"the only shortfall declared in tools/devctl/retention_floor.go is artifact=%s trace=%s (%s)",
			declared.artifact, declared.trace, declared.tracked)
	}
	return []string{fmt.Sprintf(
		"retention-floor: 02:NFR-002a rule 2 requires Run Artifact retention >= the re-evaluation window "+
			"(TRACE_RETENTION), but %s stamps %s while %s states TRACE_RETENTION=%s — %s short, and %s. "+
			"A re-evaluation inside that gap reads an empty artifact manifest and the judge decides on it "+
			"(04 丙-13). Raise the literal, lower TRACE_RETENTION, or pin the new pair here and record in "+
			"05 which work item carries it",
		artifactWhere, artifact, envExampleDoc, trace, trace-artifact, pinned)}
}

// runArtifactRetention finds the retention the run-output manifest INSERT stamps
// on expires_at. Every way of not finding exactly one is a problem, because the
// alternative is comparing nothing and reporting success.
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
			// Anchored to what the statement does, never to its name: the name
			// has already changed once under this check's feet.
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

// traceRetention reads the re-evaluation window's only stated value. Two
// assignments mean two answers, which is worse than none.
func traceRetention(root string) (problems []string, retention time.Duration) {
	data, err := os.ReadFile(filepath.Join(root, envExampleDoc))
	if err != nil {
		return []string{fmt.Sprintf("retention-floor: cannot read %s: %v", envExampleDoc, err)}, 0
	}
	var values []string
	var lines []int
	for i, line := range strings.Split(string(data), "\n") {
		if m := envTraceRetention.FindStringSubmatch(line); m != nil {
			values = append(values, m[1])
			lines = append(lines, i+1)
		}
	}
	switch len(values) {
	case 1:
	case 0:
		return []string{fmt.Sprintf(
			"retention-floor: %s no longer assigns TRACE_RETENTION; it is the only stated value of the "+
				"re-evaluation window 02:NFR-002a rule 2 measures against, so this check has lost half "+
				"its subject", envExampleDoc)}, 0
	default:
		return []string{fmt.Sprintf(
			"retention-floor: %s assigns TRACE_RETENTION %d times (lines %v); the re-evaluation window "+
				"cannot have two values", envExampleDoc, len(values), lines)}, 0
	}

	parsed, err := time.ParseDuration(values[0])
	if err != nil {
		return []string{fmt.Sprintf(
			"retention-floor: %s:%d TRACE_RETENTION=%q is not a Go duration (%v); cmd/maintenance is "+
				"fail-closed on it, so rotate-partitions is already refusing to run",
			envExampleDoc, lines[0], values[0], err)}, 0
	}
	if parsed <= 0 {
		return []string{fmt.Sprintf(
			"retention-floor: %s:%d TRACE_RETENTION=%q is not positive",
			envExampleDoc, lines[0], values[0])}, 0
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

// sqlStatements splits a sqlc query file on its `-- name:` headers and keeps
// only the non-comment lines of each body. Dropping the comments is what stops
// the prose from voting: runs.sql discusses `kind = 'run_output'` in three
// separate comment blocks, and a comment that also mentioned an interval would
// otherwise read as the statement stamping one.
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
