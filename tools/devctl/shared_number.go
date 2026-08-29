package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// One number, several files.
//
// Some values have to be identical in places no compiler compares: a Go const,
// a maxLength in the OpenAPI contract, a Pydantic max_length in apps/llm, a
// constant in a measurement harness. Nothing links them, so they drift, and the
// drift is discovered at the worst moment.
//
// The case this was written for: `maxDigestEntry` was raised from 2000 to 8000
// in judge.go alone (`04` 丙-47). Every judgement then came back 422, because
// apps/llm was still rejecting anything longer than 2000. That was the LOUDEST
// of the copies failing — the harness in tools/eval-regression carries the same
// number and, when it drifts, simply builds a different request and reports the
// results as if nothing had happened.
//
// So: each site carries a marker naming the invariant, and this check fails
// when the marked lines disagree. Same shape as the drift markers in
// automation_check.go, and chosen for the same reason — a mechanical comparison
// of things a human would otherwise have to remember are related.
//
//	Go      maxDigestEntry = 8000 // one-number: <name>
//	YAML    maxLength: 8000       # one-number: <name>
//	Python  max_length=8000)      # one-number: <name>
//
// The number and the marker are on the same line, always. That is what makes
// this readable in four syntaxes without a parser for any of them: the value is
// the last integer before the comment.
//
// `<name>` in those examples is a placeholder and stays one: the pattern below
// requires a word character straight after the colon, so the illustrations
// cannot vote. They did on the first run, and again on the second when this
// paragraph was added -- which is at least the check working — three lines of this comment counted
// as three real sites, which is the same trap driftMarkerPattern's `\b` exists
// to close, found the same way.
//
// THE MARKER OPENS THE COMMENT. `# one-number: <name> - because y` is a site;
// `# because y; one-number: <name>` is not, and it will be silently invisible rather
// than wrong. That is the second thing the first run caught: the harness in
// tools/eval-regression had its marker in the middle of an existing comment and
// did not count -- precisely the copy whose drift is otherwise silent. The rule
// is strict on purpose: finding where a comment starts in four syntaxes is the
// parser this check exists to avoid needing.
//
// This does NOT replace the contract as the source of truth (iron rule 12). The
// right fix for each of these is to generate or derive it, and where that is
// cheap it should still be done — packages/api-stub-py already derives its copy
// from the contract, which is why it carries no marker. This is for the copies
// that a generator cannot reach.
var sharedNumberMarker = regexp.MustCompile(`(?://|#)\s*one-number:\s*([A-Za-z0-9_.-]+)`)

// The last integer before the marker comment. Anchored to the end so that
// `Field(None, max_length=8000)` yields 8000 and not the 0 in some earlier
// argument.
//
// Underscores are part of the number, not a boundary: Python and Go both write
// 40_000, and the first version of this pattern read that as 000 — three sites
// agreeing on 40000 and one "disagreeing" at 000. A comparison that reports a
// difference nobody made is worse than no comparison, because the next person
// learns to distrust it.
var trailingIntPattern = regexp.MustCompile(`([0-9][0-9_]*)[^0-9]*$`)

// Where to look. Deliberately a short list of source trees rather than the whole
// repo: vendored Python under apps/llm/.venv is large, and generated output is
// not a place anyone edits a number by hand.
var sharedNumberRoots = []string{"apps", "contracts", "db", "tools"}

var sharedNumberSkip = []string{
	".venv", "node_modules", ".devctl", "site-packages",
	string(filepath.Separator) + "gen" + string(filepath.Separator),
	string(filepath.Separator) + "generated" + string(filepath.Separator),
}

type sharedNumberSite struct {
	file  string
	line  int
	value string
}

// The invariants that are supposed to exist.
//
// Everything below is built from what the scan found, which means an invariant
// whose markers ALL disappear produces no key, no loop iteration and no failure.
// That is not hypothetical here: the comment above records a copy in
// tools/eval-regression that stopped being counted because of where its marker
// sat inside a comment — one more of those and the name is simply gone, silently
// and permanently. "Only one site left" is caught; "no sites left" was not.
//
// So the names are a roster and the comparison runs both ways: a rostered name
// with no sites fails, and a marked name that is not on the roster fails. The
// second half is what keeps the roster honest — adding an invariant is an edit
// here, in the same commit as the markers, which is the same discipline the
// depguard deny lists and db/query-owners.yaml already ask for.
var sharedNumberRoster = []string{
	// The pgvector column width. Its third copy is db/migrations/0007_search.sql,
	// which cannot carry a marker because an applied migration is history; the
	// embedding-dims check reads that one and compares it with these.
	"embeddingDimensions",
	"excerptLimit",
	"generateFailureLimit",
	"generateMaxAttempts",
	"generateMaxExtraFiles",
	"generateMaxFileChars",
	"generateMaxOutputTokens",
	"generateMaxPathChars",
	"generateMaxTaskRunes",
	"judgeMaxCriterionResults",
	"judgeMaxEvidenceRefs",
	"judgeMaxQuote",
	"judgeMaxReason",
	"judgeMaxSummary",
	"maxArtifactRows",
	"maxCriteria",
	"maxDigestCount",
	"maxDigestEntry",
	"maxFinalOutput",
	// suggestCriteriaMaxItems, suggestMaxTargetFileChars and suggestMaxTargetFiles
	// were marked in apps/llm on 2026-08-25 and have one site each so far; the
	// existing "only one marked site" rule already says so.
	"suggestCriteriaMaxItems",
	"suggestMaxDigestChars",
	"suggestMaxEvidence",
	"suggestMaxExpectedImpact",
	"suggestMaxFileTreeEntries",
	"suggestMaxProblem",
	"suggestMaxProposedContent",
	"suggestMaxSuggestions",
	"suggestMaxTargetFileChars",
	"suggestMaxTargetFiles",
	"suggestMaxTargetPath",
}

// sharedNumberOwnTest names the fixtures of this check's own tests, and nothing
// else. Exact directory, exact suffix: `tools/devctl/*_test.go`.
func sharedNumberOwnTest(relative string) bool {
	relative = filepath.ToSlash(relative)
	return strings.HasPrefix(relative, "tools/devctl/") &&
		strings.HasSuffix(relative, "_test.go") &&
		!strings.Contains(strings.TrimPrefix(relative, "tools/devctl/"), "/")
}

func sharedNumberProblems(root string) []string {
	return sharedNumberProblemsFor(root, sharedNumberRoster)
}

func sharedNumberProblemsFor(root string, roster []string) []string {
	found, problems := sharedNumberScan(root)
	return append(problems, sharedNumberComparison(found, roster)...)
}

// sharedNumberScan is the walk, split out so a test can ask what it counted
// rather than only what it complained about. A fixture that leaks into the real
// scan without causing a disagreement is still a fixture voting on production,
// and only the site list can see that.
func sharedNumberScan(root string) (map[string][]sharedNumberSite, []string) {
	found := map[string][]sharedNumberSite{}
	var problems []string

	for _, tree := range sharedNumberRoots {
		err := filepath.WalkDir(filepath.Join(root, tree), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // an unreadable subtree is not this check's business
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			for _, skip := range sharedNumberSkip {
				if strings.Contains(string(filepath.Separator)+rel+string(filepath.Separator), skip) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if d.IsDir() {
				return nil
			}
			// Not this package's own tests. shared_number_test.go writes marked
			// lines with real invariant names to prove the comparison works, and
			// counting them made this check argue with production about what
			// maxDigestEntry is — the same trap the `<name>` placeholders in the
			// comment above exist to avoid, arriving from the other direction.
			//
			// Deliberately NOT "every _test.go": a test asserting a duplicated
			// limit is a legitimate copy, and an exclusion wide enough to drop it
			// is the failure this file already records once, where the
			// eval-regression copy stopped being counted. Same shape and same
			// reason as doc_identifiers.go excluding exactly its own source.
			if sharedNumberOwnTest(rel) {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".py", ".yaml", ".yml", ".sql", ".ts", ".tsx":
			default:
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for i, line := range strings.Split(string(data), "\n") {
				m := sharedNumberMarker.FindStringSubmatchIndex(line)
				if m == nil {
					continue
				}
				name := line[m[2]:m[3]]
				before := line[:m[0]]
				value := trailingIntPattern.FindStringSubmatch(before)
				if value == nil {
					problems = append(problems, fmt.Sprintf(
						"%s:%d: one-number: %s marks a line with no number on it", rel, i+1, name))
					continue
				}
				found[name] = append(found[name], sharedNumberSite{
					file: rel, line: i + 1,
					value: strings.ReplaceAll(value[1], "_", ""),
				})
			}
			return nil
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", tree, err))
		}
	}
	return found, problems
}

func sharedNumberComparison(found map[string][]sharedNumberSite, roster []string) []string {
	var problems []string
	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)

	expected := map[string]bool{}
	for _, name := range roster {
		expected[name] = true
		if len(found[name]) == 0 {
			problems = append(problems, fmt.Sprintf(
				"one-number: %s is on the roster in tools/devctl/shared_number.go but no marked site was found; "+
					"either every marker fell off (the value is still duplicated and nothing compares the copies) "+
					"or the invariant is gone and the roster entry should go with it", name))
		}
	}
	for _, name := range names {
		if !expected[name] {
			problems = append(problems, fmt.Sprintf(
				"one-number: %s is marked at %d site(s) but is not on the roster in tools/devctl/shared_number.go; "+
					"add it there so losing every marker later is a failure and not a silence", name, len(found[name])))
		}
	}

	for _, name := range names {
		sites := found[name]
		// A marker on a single site protects nothing, and the way it gets there
		// is somebody deleting the other copies' markers rather than the copies.
		if len(sites) < 2 {
			problems = append(problems, fmt.Sprintf(
				"one-number: %s has only one marked site (%s:%d); either the others lost their markers "+
					"or the value is no longer duplicated and the marker should go",
				name, sites[0].file, sites[0].line))
			continue
		}
		distinct := map[string]bool{}
		for _, s := range sites {
			distinct[s.value] = true
		}
		if len(distinct) > 1 {
			var where []string
			for _, s := range sites {
				where = append(where, fmt.Sprintf("%s:%d=%s", s.file, s.line, s.value))
			}
			problems = append(problems, fmt.Sprintf(
				"one-number: %s disagrees across %d sites: %s", name, len(sites), strings.Join(where, " ")))
		}
	}
	return problems
}
