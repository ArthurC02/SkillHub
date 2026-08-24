package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// What the depguard deny lists actually say, checked against ADR-032 appendix A.
//
// The lint config claims, at its own top, that the two sides are kept in step by
// `devctl automation-check`. What was compared was the multiset of `drift: DDD-n`
// markers — and both sides hold zero real markers, because the last tolerated
// drift was cleared. So the comparison was 0 == 0 and stayed 0 == 0 no matter
// what the rules underneath it said.
//
// That matters because of how permissions are granted here. The rules are DENY
// lists with "legal but unlisted = denied" already folded in, so **adding a
// cross-context permission is performed by deleting two lines**:
//
//   - pkg: github.com/.../internal/skill/discovery
//     desc: "cross-context import forbidden by ADR-032; see ADR-032 appendix A"
//
// Delete that from the `identity` rule and `creator/workspace` may import
// `skill/discovery` freely. No marker was added or removed, so drift-marker
// passed. contextMapProblems passes too: it only asks "does a rule exist for
// this path", never what the rule contains. And appendix A is untouched, so the
// human whitelist still says the import is forbidden. Nothing anywhere was red.
//
// So this reads the deny contents. For every rule that guards one Core or
// Supporting context, the deny set must equal
//
//	(all context boundaries + apiserver + objreconcile) - self - permitted
//
// where `permitted` is parsed out of appendix A's table. Both directions fail,
// with one deliberate asymmetry described at wildcardGrantee below.
//
// NOT in scope: the `generic`, `shared-kernel` and `objreconcile` rules. Their
// permitted sets are not written in appendix A at all (ADR-032 §1 and ADR-037
// govern them, in prose), so arithmetic against the appendix would be arithmetic
// against a document that does not answer the question. They are named here so
// the gap is a stated limit rather than a silence.
const denyPackagePrefix = "github.com/ArthurC02/skillhub/apps/platform/internal/"

// The two non-context packages every context rule must also deny. Neither is a
// bounded context, so neither comes out of §1's Core/Supporting rows, and both
// are denied by every single rule in the file today: `apiserver` is the
// composition root (ADR-032 §5, "contexts must not import it") and
// `objreconcile` is a generic worker (appendix A). Hard-coded rather than
// inferred from the rules themselves, because inferring the universe from the
// deny lists is how a check ends up agreeing with whatever the deny lists say.
var alwaysDenied = []string{"apiserver", "objreconcile"}

var (
	appendixHeading = "## 附錄 A"
	// A backticked lower-case identifier: the appendix writes every Boundary ID
	// that way, and writes everything else (「全部 context」,「各 context」) plain.
	appendixID = regexp.MustCompile("`([a-z][a-z0-9_]*)`")
	// `        identity:` — a rule name under depguard's `rules:`.
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

	// The universe a context rule chooses from, and which of its members are
	// contexts at all.
	universe := map[string]bool{}
	for id, identity := range declared {
		if identity.Kind == architectureCore || identity.Kind == architectureSupporting {
			universe[id] = true
		}
	}
	for _, id := range alwaysDenied {
		if !knownBoundaryID(declared, id) {
			problems = append(problems, fmt.Sprintf(
				"depguard-deny: %s §1 has no Boundary ID %q, which every context rule is required to deny", adrPath, id))
			continue
		}
		universe[id] = true
	}

	rules := depguardRules(string(lint))
	checked := 0
	for _, rule := range sortedKeys(rules) {
		files, deny := rules[rule]["files"], rules[rule]["deny"]
		// Only single-selector rules that guard one Core/Supporting context: the
		// multi-selector `generic` rule and the Shared Kernel / Generic ones are
		// governed elsewhere (see the file comment).
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
			identity, known := resolveContextPath(path, declared)
			if !known {
				problems = append(problems, fmt.Sprintf(
					"depguard-deny: %s rule %q denies internal/%s, which no %s §1 row declares", lintPath, rule, path, contextMapADR))
				continue
			}
			denied[identity.ID] = true
		}

		// The asymmetry: a rule may refuse a BLANKET grant (`各 context → identity`)
		// and the `policy` rule does, on purpose — "it must be able to answer
		// without knowing who is asking". Refusing a blanket grant is stricter
		// than the appendix and cannot open a hole, so it is not reported.
		// Refusing a NAMED pair is reported: that is a rule and an appendix row
		// disagreeing about one specific collaboration.
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
	sort.Strings(problems)
	return problems
}

// appendixPermissions reads appendix A's table. A row is `| A → B、C | … | 保留 |`:
// the left of the arrow names the importer, the right names what it may import,
// and the last cell must still say 保留 — a dependency moved out of the whitelist
// stops being a permission at that moment, which is what the appendix's own
// "移出白名單的項目不得再加回" says.
//
// A row whose left side has no backticked id (`各 context → identity`) is the
// blanket grant; a row whose right side has none (`apiserver` → 全部 context) is
// about a package with no rule of its own and is skipped.
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
		// 保留, possibly with a parenthetical (「保留（trace 改注入，DDD-004）」).
		// A row that stopped saying 保留 stopped being a permission, which is what
		// the appendix's own 「移出白名單的項目不得再加回」 means.
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
		if len(sources) == 0 { // 各 context → `identity`
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

// depguardRules reads the `files:` and `deny:` lists of every rule under
// `linters.settings.depguard.rules`. Indentation-anchored rather than a real
// YAML parser, for the reason parseOwnerDeclaration gives: devctl has no
// dependencies and this shape does not justify the first one. Anything that does
// not match the shape is not a rule and is ignored — which is why the comments
// come off first (see stripYAMLComments).
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

// soleContextOfRule maps a rule's `files:` list to the one context it guards.
// `!$test` is depguard's own exclusion token, not a path; more than one real
// selector means the rule covers a group (`generic`) and has no single self.
func soleContextOfRule(files []string, declared map[string]packageIdentity) (string, bool) {
	var ids []string
	for _, selector := range files {
		m := depguardFilePattern.FindStringSubmatch(selector)
		if m == nil {
			continue
		}
		if identity, ok := resolveContextPath(m[1], declared); ok {
			ids = append(ids, identity.ID)
		}
	}
	if len(ids) != 1 {
		return "", false
	}
	return ids[0], true
}

// stripYAMLComments removes trailing `#` comments, honouring quotes.
//
// Without it, any `**/internal/x/y/**` written in prose votes: the collector in
// contextMapProblems runs its regex over the whole file text, so commenting a
// rule out while debugging leaves the path still "guarded" as far as the check
// is concerned, and .golangci.yml already carries prose about which contexts may
// reach which. The quote tracking is not decoration — a `desc:` value legitimately
// contains `#` in a URL fragment or an issue number, and cutting there would
// truncate a rule rather than a comment.
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
