package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const denyPackagePrefix = "github.com/ArthurC02/skillhub/apps/platform/internal/"

const objreconcileContextID = "objreconcile"

const depguardFilesKey = "files"

var compositionRoots = []string{"apiserver", "worker", "wiring"}

var alwaysDenied = append(slices.Clone(compositionRoots), objreconcileContextID)

func isCompositionRoot(id string) bool { return slices.Contains(compositionRoots, id) }

var (
	depguardRuleName = regexp.MustCompile(`^ {8}([A-Za-z0-9_-]+):\s*$`)
	depguardListKey  = regexp.MustCompile(`^ {10}(files|deny):\s*$`)
	depguardDenyPkg  = regexp.MustCompile(`^ {12}- pkg:\s*(\S+)\s*$`)
	depguardFileItem = regexp.MustCompile(`^ {12}- "([^"]*)"\s*$`)
)

func depguardDenyProblems(root string) []string {
	const lintPath = "apps/platform/.golangci.yml"

	declared, problems := architectureIdentities(root)
	if len(declared) == 0 {
		return append(problems, fmt.Sprintf("depguard-deny: %s declares no contexts; this check has lost its subject", identityHomes))
	}
	lint, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lintPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("depguard-deny: %s: %v", lintPath, err))
	}

	permitted, policies, problems := reviewedDependencyPermissions(root, problems)
	if policies == 0 {
		return append(problems, fmt.Sprintf(
			"depguard-deny: %s holds no reviewed dependency policy; this check has lost its subject", dependencyPoliciesFile))
	}

	rules := depguardRules(string(lint))
	problems = append(problems, depguardSelectorProblems(rules, declared, lintPath)...)
	audit := depguardDenyAudit{
		lintPath:  lintPath,
		declared:  declared,
		permitted: permitted,
		universe:  boundedContextsAndAlwaysDenied(declared),
		pathIDs:   contextIDsByPath(declared),
	}
	checked := 0
	for _, rule := range sortedKeys(rules) {
		guarded, ruleProblems := audit.ruleProblems(rule, rules[rule][depguardFilesKey], rules[rule]["deny"])
		if guarded {
			checked++
		}
		problems = append(problems, ruleProblems...)
	}
	if checked == 0 {
		problems = append(problems, fmt.Sprintf(
			"depguard-deny: %s has no depguard rule guarding a single Core/Supporting context; this check has lost its subject",
			lintPath))
	}
	problems = append(problems, specialDepguardProblems(rules, declared, lintPath)...)
	sort.Strings(problems)
	return problems
}

func boundedContextsAndAlwaysDenied(declared map[string]packageIdentity) map[string]bool {
	universe := map[string]bool{}
	for id, identity := range declared {
		if isBoundedContextKind(identity.Kind) {
			universe[id] = true
		}
	}
	for _, id := range alwaysDenied {
		if knownBoundaryID(declared, id) {
			universe[id] = true
		}
	}
	return universe
}

func contextIDsByPath(declared map[string]packageIdentity) map[string]string {
	pathIDs := map[string]string{}
	for id, identity := range declared {
		pathIDs[strings.TrimSuffix(identity.Path, "/*")] = id
	}
	return pathIDs
}

type depguardDenyAudit struct {
	lintPath  string
	declared  map[string]packageIdentity
	permitted map[string]map[string]bool
	universe  map[string]bool
	pathIDs   map[string]string
}

func (a depguardDenyAudit) ruleProblems(rule string, files, deny []string) (bool, []string) {
	self, ok := soleContextOfRule(files, a.declared)
	if !ok || !isBoundedContextKind(a.declared[self].Kind) {
		return false, nil
	}
	denied, problems := a.deniedContexts(rule, deny)
	for _, target := range sortedKeys(a.universe) {
		switch {
		case target == self:
		case denied[target] && a.permitted[self][target]:
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s rule %q denies %q, but %s keeps `%s` → `%s`; "+
					"the two sides disagree about that collaboration",
				a.lintPath, rule, target, dependencyPoliciesFile, self, target))
		case !denied[target] && !a.permitted[self][target]:
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s rule %q does not deny %q and %s does not permit `%s` → `%s`; "+
					"a deletion from a deny list IS a new permission (\"legal but unlisted = denied\"), so add the "+
					"appendix row or restore the deny entry",
				a.lintPath, rule, target, dependencyPoliciesFile, self, target))
		}
	}
	return true, problems
}

func (a depguardDenyAudit) deniedContexts(rule string, deny []string) (map[string]bool, []string) {
	denied := map[string]bool{}
	var problems []string
	for _, pkg := range deny {
		path, found := strings.CutPrefix(pkg, denyPackagePrefix)
		if !found {
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s rule %q denies %q, which is not an apps/platform/internal package", a.lintPath, rule, pkg))
			continue
		}
		id, known := a.pathIDs[path]
		if !known {
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s rule %q denies internal/%s, which is not an exact %s package path", a.lintPath, rule, path, identityHomes))
			continue
		}
		denied[id] = true
	}
	return denied, problems
}

func isBoundedContextKind(kind architectureKind) bool {
	return kind == architectureCore || kind == architectureSupporting
}

type depguardMembershipVerb struct {
	base        string
	thirdPerson string
}

var (
	depguardDenies  = depguardMembershipVerb{base: "deny", thirdPerson: "denies"}
	depguardSelects = depguardMembershipVerb{base: "select", thirdPerson: "selects"}
)

func depguardMembershipProblems(lintPath, rule string, want, actual map[string]bool, verb depguardMembershipVerb) []string {
	var problems []string
	for _, id := range sortedKeys(want) {
		if !actual[id] {
			problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q does not %s %q", lintPath, rule, verb.base, id))
		}
	}
	for _, id := range sortedKeys(actual) {
		if !want[id] {
			problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q unexpectedly %s %q", lintPath, rule, verb.thirdPerson, id))
		}
	}
	return problems
}

func specialDepguardProblems(rules map[string]map[string][]string, declared map[string]packageIdentity, lintPath string) []string {

	if _, ok := declared["worker"]; !ok {
		return nil
	}
	bounded := map[string]bool{}
	pathIDs := map[string]string{}
	for id, identity := range declared {
		pathIDs[strings.TrimSuffix(identity.Path, "/*")] = id
		if identity.Kind == architectureCore || identity.Kind == architectureSupporting {
			bounded[id] = true
		}
	}
	expected := map[string]map[string]bool{
		"shared-kernel":       copySet(bounded),
		"generic":             copySet(bounded),
		objreconcileContextID: copySet(bounded),
	}
	for _, id := range alwaysDenied {
		if _, ok := declared[id]; !ok {
			continue
		}
		expected["generic"][id] = true
		expected["shared-kernel"][id] = true
		if id != objreconcileContextID {
			expected[objreconcileContextID][id] = true
		}
	}

	var problems []string
	for _, rule := range []string{"generic", objreconcileContextID, "shared-kernel"} {
		actual := map[string]bool{}
		for _, pkg := range rules[rule]["deny"] {
			path, ok := strings.CutPrefix(pkg, denyPackagePrefix)
			if !ok {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q denies non-platform package %q", lintPath, rule, pkg))
				continue
			}
			id, ok := pathIDs[path]
			if !ok {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q denies internal/%s, which is not an exact %s package path", lintPath, rule, path, identityHomes))
				continue
			}
			actual[id] = true
		}
		problems = append(problems, depguardMembershipProblems(lintPath, rule, expected[rule], actual, depguardDenies)...)
	}
	return problems
}

func depguardSelectorProblems(rules map[string]map[string][]string, declared map[string]packageIdentity, lintPath string) []string {
	pathIDs := contextIDsByPath(declared)
	var problems []string
	for rule, want := range expectedDepguardSelections(declared) {
		actual, selectorProblems := selectedContexts(lintPath, rule, rules[rule][depguardFilesKey], pathIDs)
		problems = append(problems, selectorProblems...)
		problems = append(problems, depguardMembershipProblems(lintPath, rule, want, actual, depguardSelects)...)
	}
	return problems
}

func expectedDepguardSelections(declared map[string]packageIdentity) map[string]map[string]bool {
	expected := map[string]map[string]bool{}
	for id, identity := range declared {
		switch {
		case isBoundedContextKind(identity.Kind):
			expected[id] = map[string]bool{id: true}
		case identity.Kind == architectureSharedKernel:
			if expected["shared-kernel"] == nil {
				expected["shared-kernel"] = map[string]bool{}
			}
			expected["shared-kernel"][id] = true
		case identity.Kind == architectureGeneric && id != "api" && id != objreconcileContextID && !isCompositionRoot(id):
			if expected["generic"] == nil {
				expected["generic"] = map[string]bool{}
			}
			expected["generic"][id] = true
		case id == objreconcileContextID:
			expected[objreconcileContextID] = map[string]bool{id: true}
		}
	}
	return expected
}

func selectedContexts(lintPath, rule string, selectors []string, pathIDs map[string]string) (map[string]bool, []string) {
	actual := map[string]bool{}
	var problems []string
	for _, selector := range selectors {
		if selector == "!$test" {
			continue
		}
		match := depguardSelectorPattern.FindStringSubmatch(selector)
		if match == nil {
			problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q has unrecognised %s selector %q", lintPath, rule, depguardFilesKey, selector))
			continue
		}
		id, known := pathIDs[match[1]]
		if !known {
			problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q selector internal/%s is not an exact %s package path", lintPath, rule, match[1], identityHomes))
			continue
		}
		actual[id] = true
	}
	return actual, problems
}

func copySet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key := range source {
		result[key] = true
	}
	return result
}

// Reads rule blocks by fixed indentation level rather than a full YAML parser.
func depguardRules(lint string) map[string]map[string][]string {
	rules := map[string]map[string][]string{}
	rule, key := "", ""
	for _, line := range strings.Split(stripYAMLComments(lint), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := depguardRuleName.FindStringSubmatch(line); m != nil {
			rule, key = m[1], ""
			rules[rule] = map[string][]string{}
			continue
		}
		if rule == "" {
			continue
		}
		if m := depguardListKey.FindStringSubmatch(line); m != nil {
			key = m[1]
			continue
		}
		switch key {
		case depguardFilesKey:
			if m := depguardFileItem.FindStringSubmatch(line); m != nil {
				rules[rule][depguardFilesKey] = append(rules[rule][depguardFilesKey], m[1])
			}
		case "deny":
			if m := depguardDenyPkg.FindStringSubmatch(line); m != nil {
				rules[rule]["deny"] = append(rules[rule]["deny"], m[1])
			}
		}
	}
	return rules
}

func soleContextOfRule(files []string, declared map[string]packageIdentity) (string, bool) {
	ids := map[string]bool{}
	for _, selector := range files {
		// "!$test" is depguard's own exclusion token, not a path, so it never
		// matches here and is skipped rather than resolved.
		m := depguardSelectorPattern.FindStringSubmatch(selector)
		if m == nil {
			continue
		}
		if identity, ok := resolveContextPath(m[1], declared); ok {
			ids[identity.ID] = true
		}
	}
	if len(ids) != 1 {
		return "", false
	}
	for id := range ids {
		return id, true
	}
	return "", false
}

// Tracks open quotes so a "#" inside a quoted value isn't cut as a comment.
func stripYAMLComments(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		var quote byte
		for j := 0; j < len(line); j++ {
			switch c := line[j]; {
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case c == '\'' || c == '"':
				quote = c
			case c == '#':
				line = line[:j]
				j = len(line)
			}
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func reviewedDependencyPermissions(root string, problems []string) (map[string]map[string]bool, int, []string) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dependencyPoliciesFile)))
	if err != nil {
		return nil, 0, append(problems, fmt.Sprintf("depguard-deny: %s: %v", dependencyPoliciesFile, err))
	}
	var doc struct {
		Dependencies []struct {
			ID     string `json:"id"`
			From   string `json:"from_context"`
			To     string `json:"to_context"`
			Policy string `json:"policy"`
			Status string `json:"status"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, 0, append(problems, fmt.Sprintf("depguard-deny: %s: %v", dependencyPoliciesFile, err))
	}
	permitted := map[string]map[string]bool{}
	counted := 0
	for _, entry := range doc.Dependencies {
		if entry.Status != "reviewed" {
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s:%s is %q; only a reviewed policy permits a collaboration",
				dependencyPoliciesFile, entry.ID, entry.Status))
			continue
		}
		counted++
		if entry.Policy != "allowed" {
			continue
		}
		if permitted[entry.From] == nil {
			permitted[entry.From] = map[string]bool{}
		}
		permitted[entry.From][entry.To] = true
	}
	return permitted, counted, problems
}
