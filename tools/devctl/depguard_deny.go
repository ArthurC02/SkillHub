package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const denyPackagePrefix = "github.com/ArthurC02/skillhub/apps/platform/internal/"

var compositionRoots = []string{"apiserver", "worker", "wiring"}

var alwaysDenied = append(slices.Clone(compositionRoots), "objreconcile")

func isCompositionRoot(id string) bool { return slices.Contains(compositionRoots, id) }

var (
	appendixHeading = "## 附錄 A"

	// A backticked lowercase identifier; the appendix writes everything else
	// (plain prose) unbacked.
	appendixID = regexp.MustCompile("`([a-z][a-z0-9_]*)`")

	depguardRuleName = regexp.MustCompile(`^ {8}([A-Za-z0-9_-]+):\s*$`)
	depguardListKey  = regexp.MustCompile(`^ {10}(files|deny):\s*$`)
	depguardDenyPkg  = regexp.MustCompile(`^ {12}- pkg:\s*(\S+)\s*$`)
	depguardFileItem = regexp.MustCompile(`^ {12}- "([^"]*)"\s*$`)
)

func depguardDenyProblems(root string) []string {
	const adrPath, lintPath = "docs/adr/" + contextMapADR, "apps/platform/.golangci.yml"

	declared, problems := contextTablePackages(filepath.Join(root, filepath.FromSlash(adrPath)), adrPath)
	if len(declared) == 0 {
		return append(problems, fmt.Sprintf("depguard-deny: %s §1 declares no contexts; this check has lost its subject", adrPath))
	}
	adr, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adrPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("depguard-deny: %s: %v", adrPath, err))
	}
	lint, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lintPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("depguard-deny: %s: %v", lintPath, err))
	}

	permitted, wildcard, appendixRows := appendixPermissions(string(adr), declared)
	if appendixRows == 0 {
		return append(problems, fmt.Sprintf(
			"depguard-deny: %s %s has no `A → B` rows; this check has lost its subject", adrPath, appendixHeading))
	}

	universe := map[string]bool{}
	for id, identity := range declared {
		if identity.Kind == architectureCore || identity.Kind == architectureSupporting {
			universe[id] = true
		}
	}
	for _, id := range alwaysDenied {
		if !knownBoundaryID(declared, id) {
			continue
		}
		universe[id] = true
	}

	rules := depguardRules(string(lint))
	problems = append(problems, depguardSelectorProblems(rules, declared, lintPath)...)
	checked := 0
	pathIDs := map[string]string{}
	for id, identity := range declared {
		pathIDs[strings.TrimSuffix(identity.Path, "/*")] = id
	}
	for _, rule := range sortedKeys(rules) {
		files, deny := rules[rule]["files"], rules[rule]["deny"]

		self, ok := soleContextOfRule(files, declared)
		if !ok || (declared[self].Kind != architectureCore && declared[self].Kind != architectureSupporting) {
			continue
		}
		checked++

		denied := map[string]bool{}
		for _, pkg := range deny {
			path, found := strings.CutPrefix(pkg, denyPackagePrefix)
			if !found {
				problems = append(problems, fmt.Sprintf(
					"depguard-deny: %s rule %q denies %q, which is not an apps/platform/internal package", lintPath, rule, pkg))
				continue
			}
			id, known := pathIDs[path]
			if !known {
				problems = append(problems, fmt.Sprintf(
					"depguard-deny: %s rule %q denies internal/%s, which is not an exact %s §1 package path", lintPath, rule, path, contextMapADR))
				continue
			}
			denied[id] = true
		}

		for _, target := range sortedKeys(universe) {
			switch {
			case target == self:
			case denied[target] && permitted[self][target]:
				problems = append(problems, fmt.Sprintf(
					"depguard-deny: %s rule %q denies %q, but %s appendix A keeps `%s` → `%s`; "+
						"the two sides disagree about that collaboration",
					lintPath, rule, target, contextMapADR, self, target))
			case !denied[target] && !permitted[self][target] && !wildcard[target]:
				problems = append(problems, fmt.Sprintf(
					"depguard-deny: %s rule %q does not deny %q and %s appendix A does not permit `%s` → `%s`; "+
						"a deletion from a deny list IS a new permission (\"legal but unlisted = denied\"), so add the "+
						"appendix row or restore the deny entry",
					lintPath, rule, target, contextMapADR, self, target))
			}
		}
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
		"shared-kernel": copySet(bounded),
		"generic":       copySet(bounded),
		"objreconcile":  copySet(bounded),
	}
	for _, id := range alwaysDenied {
		if _, ok := declared[id]; !ok {
			continue
		}
		expected["generic"][id] = true
		expected["shared-kernel"][id] = true
		if id != "objreconcile" {
			expected["objreconcile"][id] = true
		}
	}

	var problems []string
	for _, rule := range []string{"generic", "objreconcile", "shared-kernel"} {
		actual := map[string]bool{}
		for _, pkg := range rules[rule]["deny"] {
			path, ok := strings.CutPrefix(pkg, denyPackagePrefix)
			if !ok {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q denies non-platform package %q", lintPath, rule, pkg))
				continue
			}
			id, ok := pathIDs[path]
			if !ok {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q denies internal/%s, which is not an exact ADR-032 §1 package path", lintPath, rule, path))
				continue
			}
			actual[id] = true
		}
		for _, id := range sortedKeys(expected[rule]) {
			if !actual[id] {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q does not deny %q", lintPath, rule, id))
			}
		}
		for _, id := range sortedKeys(actual) {
			if !expected[rule][id] {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q unexpectedly denies %q", lintPath, rule, id))
			}
		}
	}
	return problems
}

func depguardSelectorProblems(rules map[string]map[string][]string, declared map[string]packageIdentity, lintPath string) []string {
	expected := map[string]map[string]bool{}
	pathIDs := map[string]string{}
	for id, identity := range declared {
		pathIDs[strings.TrimSuffix(identity.Path, "/*")] = id
		switch {
		case identity.Kind == architectureCore || identity.Kind == architectureSupporting:
			expected[id] = map[string]bool{id: true}
		case identity.Kind == architectureSharedKernel:
			if expected["shared-kernel"] == nil {
				expected["shared-kernel"] = map[string]bool{}
			}
			expected["shared-kernel"][id] = true
		case identity.Kind == architectureGeneric && id != "api" && id != "objreconcile" && !isCompositionRoot(id):
			if expected["generic"] == nil {
				expected["generic"] = map[string]bool{}
			}
			expected["generic"][id] = true
		case id == "objreconcile":
			expected["objreconcile"] = map[string]bool{id: true}
		}
	}

	var problems []string
	for rule, want := range expected {
		actual := map[string]bool{}
		for _, selector := range rules[rule]["files"] {
			if selector == "!$test" {
				continue
			}
			match := depguardSelectorPattern.FindStringSubmatch(selector)
			if match == nil {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q has unrecognised files selector %q", lintPath, rule, selector))
				continue
			}
			id, known := pathIDs[match[1]]
			if !known {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q selector internal/%s is not an exact ADR-032 §1 package path", lintPath, rule, match[1]))
				continue
			}
			actual[id] = true
		}
		for _, id := range sortedKeys(want) {
			if !actual[id] {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q does not select %q", lintPath, rule, id))
			}
		}
		for _, id := range sortedKeys(actual) {
			if !want[id] {
				problems = append(problems, fmt.Sprintf("depguard-deny: %s rule %q unexpectedly selects %q", lintPath, rule, id))
			}
		}
	}
	return problems
}

func copySet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key := range source {
		result[key] = true
	}
	return result
}

func appendixPermissions(adr string, declared map[string]packageIdentity) (permitted map[string]map[string]bool, wildcard map[string]bool, rows int) {
	permitted, wildcard = map[string]map[string]bool{}, map[string]bool{}
	inAppendix := false
	for _, line := range strings.Split(adr, "\n") {
		if strings.HasPrefix(line, "## ") {
			inAppendix = strings.HasPrefix(line, appendixHeading)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !inAppendix || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")

		// A row is "| A → B、C | ... | 保留 |": left of the arrow is the
		// importer, right is what it may import; a row no longer saying 保留
		// is no longer a permission.
		if len(cells) != 3 || !strings.HasPrefix(strings.TrimSpace(cells[2]), "保留") {
			continue
		}
		from, to, arrow := strings.Cut(cells[0], "→")
		if !arrow {
			continue
		}
		var sources, targets []string
		for _, m := range appendixID.FindAllStringSubmatch(from, -1) {
			if knownBoundaryID(declared, m[1]) {
				sources = append(sources, m[1])
			}
		}
		for _, m := range appendixID.FindAllStringSubmatch(to, -1) {
			if knownBoundaryID(declared, m[1]) {
				targets = append(targets, m[1])
			}
		}
		if len(targets) == 0 {
			continue
		}
		rows++
		if len(sources) == 0 {
			// A blank left side is a blanket grant to every context.
			for _, target := range targets {
				wildcard[target] = true
			}
			continue
		}
		for _, source := range sources {
			if permitted[source] == nil {
				permitted[source] = map[string]bool{}
			}
			for _, target := range targets {
				permitted[source][target] = true
			}
		}
	}
	return permitted, wildcard, rows
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
		case "files":
			if m := depguardFileItem.FindStringSubmatch(line); m != nil {
				rules[rule]["files"] = append(rules[rule]["files"], m[1])
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
