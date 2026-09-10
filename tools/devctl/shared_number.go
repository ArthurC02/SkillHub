package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sharedNumberMarker = regexp.MustCompile(`(?://|#)\s*one-number:\s*([A-Za-z0-9_.-]+)`)

var trailingIntPattern = regexp.MustCompile(`([0-9][0-9_]*)[^0-9]*$`)

var sharedNumberRoots = []string{"apps", "contracts", "db", "infra", "tools"}

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

var sharedNumberRoster = []string{

	"creationMaxDiagramBytes",
	"embeddingDimensions",
	"excerptLimit",
	"generateFailureLimit",
	"generateMaxAttempts",

	"generateMaxDiagramBytes",
	"generateMaxExtraFiles",
	"generateMaxFileChars",
	"generateMaxOutputTokens",
	"generateMaxPathChars",
	"generateMaxReferenceChars",
	"generateMaxReferences",
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
	"maxSkillPackageEntries",

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

func sharedNumberScan(root string) (map[string][]sharedNumberSite, []string) {
	found := map[string][]sharedNumberSite{}
	var problems []string

	for _, tree := range sharedNumberRoots {
		err := filepath.WalkDir(filepath.Join(root, tree), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
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

			if sharedNumberOwnTest(rel) {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".py", ".yaml", ".yml", ".sql", ".ts", ".tsx", ".mjs":
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
