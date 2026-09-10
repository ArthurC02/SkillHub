package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const denyADRTable = `### 1. Context 對照表

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| 創作者帳戶與工作區／Identity & Workspace | Core | identity | creator/workspace | WS |
| Skill 探索／Catalog & Discovery | Core | catalog | skill/discovery | DISC |
| 執行證據／Run Trace | Supporting | trace | trial/evidence | TRACE |
| — | Generic | apiserver | entrypoint/api/apiserver | — |
| — | Generic | objreconcile | foundation/storage/objreconcile | — |

### 2. 其他

## 附錄 A：跨 context import 白名單

| 依賴 | 判定 | 處置 |
| --- | --- | --- |
| ` + "`apiserver`" + ` → 全部 context | 表現層，合法 | 保留 |
| ` + "`catalog`" + ` → ` + "`trace`" + `（同步查詢） | 同步查詢，合法 | 保留（DDD-004） |
| 各 context → ` + "`identity`" + `（Workspace scope） | 鐵律 3 的入口，合法 | 保留 |
`

const denyPrefix = "github.com/ArthurC02/skillhub/apps/platform/internal/"

func rule(name, selector string, deny ...string) string {
	out := "        " + name + ":\n          files:\n            - \"" + selector + "\"\n" +
		"            - \"!$test\"\n          deny:\n"
	for _, pkg := range deny {
		out += "            - pkg: " + denyPrefix + pkg + "\n" +
			"              desc: \"cross-context import forbidden by ADR-032\"\n"
	}
	return out
}

func denyConfig() string {
	return "version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" +
		rule("identity", "**/internal/creator/workspace/**",
			"skill/discovery", "trial/evidence", "entrypoint/api/apiserver", "foundation/storage/objreconcile") +
		rule("catalog", "**/internal/skill/discovery/**",
			"entrypoint/api/apiserver", "foundation/storage/objreconcile") +
		rule("trace", "**/internal/trial/evidence/**",
			"creator/workspace", "skill/discovery", "entrypoint/api/apiserver", "foundation/storage/objreconcile") +
		rule("objreconcile", "**/internal/foundation/storage/objreconcile/**")
}

func writeDenyFixture(t *testing.T, adr, lint string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range map[string]string{
		"docs/adr/" + contextMapADR:   adr,
		"apps/platform/.golangci.yml": lint,
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDepguardDenyAcceptsRulesThatMatchAppendixA(t *testing.T) {
	t.Parallel()
	if problems := depguardDenyProblems(writeDenyFixture(t, denyADRTable, denyConfig())); len(problems) != 0 {
		t.Fatalf("a config that matches appendix A was rejected: %v", problems)
	}
}

func TestDepguardDenyRejectsAPermissionGrantedByDeletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		adr  string
		lint string
		want string
	}{{

		name: "a deny entry was deleted, which is how a permission is granted here",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			"            - pkg: "+denyPrefix+"skill/discovery\n"+
				"              desc: \"cross-context import forbidden by ADR-032\"\n", "", 1),
		want: `rule "identity" does not deny "catalog"`,
	}, {

		name: "the apiserver deny line was deleted",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			"            - pkg: "+denyPrefix+"entrypoint/api/apiserver\n"+
				"              desc: \"cross-context import forbidden by ADR-032\"\n", "", 1),
		want: `rule "identity" does not deny "apiserver"`,
	}, {

		name: "the lint denies a pair the appendix keeps",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			rule("catalog", "**/internal/skill/discovery/**",
				"entrypoint/api/apiserver", "foundation/storage/objreconcile"),
			rule("catalog", "**/internal/skill/discovery/**",
				"trial/evidence", "entrypoint/api/apiserver", "foundation/storage/objreconcile"), 1),
		want: "appendix A keeps `catalog` → `trace`",
	}, {

		name: "the appendix row was removed and the rule still allows it",
		adr:  strings.Replace(denyADRTable, "| 同步查詢，合法 | 保留（DDD-004） |", "| 事件化 | 移出 |", 1),
		lint: denyConfig(),
		want: `rule "catalog" does not deny "trace"`,
	}, {

		name: "a deny entry names a package no §1 row declares",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(), denyPrefix+"skill/discovery", denyPrefix+"skill/discovry", 1),
		want: "denies internal/skill/discovry, which is not an exact",
	}, {
		name: "a narrower child deny does not cover the context root",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(), denyPrefix+"skill/discovery", denyPrefix+"skill/discovery/nonexistent", 1),
		want: "not an exact",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := depguardDenyProblems(writeDenyFixture(t, tc.adr, tc.lint))
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestDepguardDenyStillChecksARuleWithDuplicateSelectors(t *testing.T) {
	t.Parallel()
	lint := strings.Replace(denyConfig(),
		"            - \"**/internal/creator/workspace/**\"\n",
		"            - \"**/internal/creator/workspace/**\"\n            - \"**/internal/creator/workspace/**\"\n", 1)
	lint = strings.Replace(lint,
		"            - pkg: "+denyPrefix+"skill/discovery\n"+
			"              desc: \"cross-context import forbidden by ADR-032\"\n", "", 1)
	problems := strings.Join(depguardDenyProblems(writeDenyFixture(t, denyADRTable, lint)), "\n")
	if !strings.Contains(problems, `rule "identity" does not deny "catalog"`) {
		t.Fatalf("a duplicate selector bypassed deny reconciliation: %s", problems)
	}
}

func TestDepguardDenyChecksProseGovernedRules(t *testing.T) {
	t.Parallel()
	adr := strings.Replace(denyADRTable,
		"| — | Generic | objreconcile | foundation/storage/objreconcile | — |",
		"| — | Generic | objreconcile | foundation/storage/objreconcile | — |\n"+
			"| — | Generic | worker | entrypoint/worker | — |\n"+
			"| — | Generic | audit | foundation/observability/audit | — |\n"+
			"| — | Shared Kernel | skillpkg | shared/skillpkg | — |", 1)
	bounded := []string{"creator/workspace", "skill/discovery", "trial/evidence"}
	composition := []string{"entrypoint/api/apiserver", "entrypoint/worker", "foundation/storage/objreconcile"}
	sharedDenied := append(append([]string{}, bounded...), composition...)
	sharedRule := rule("shared-kernel", "**/internal/shared/skillpkg/**", sharedDenied...)
	genericRule := rule("generic", "**/internal/foundation/observability/audit/**",
		"creator/workspace", "skill/discovery", "trial/evidence",
		"entrypoint/api/apiserver", "entrypoint/worker", "foundation/storage/objreconcile")
	objRule := rule("objreconcile", "**/internal/foundation/storage/objreconcile/**",
		"creator/workspace", "skill/discovery", "trial/evidence",
		"entrypoint/api/apiserver", "entrypoint/worker")
	identityRule := rule("identity", "**/internal/creator/workspace/**", "skill/discovery", "trial/evidence", "entrypoint/api/apiserver", "entrypoint/worker", "foundation/storage/objreconcile")
	catalogRule := rule("catalog", "**/internal/skill/discovery/**", "entrypoint/api/apiserver", "entrypoint/worker", "foundation/storage/objreconcile")
	traceRule := rule("trace", "**/internal/trial/evidence/**", "creator/workspace", "skill/discovery", "entrypoint/api/apiserver", "entrypoint/worker", "foundation/storage/objreconcile")
	coreConfig := "version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" +
		identityRule + catalogRule +
		traceRule
	config := coreConfig + sharedRule + genericRule + objRule
	if problems := depguardDenyProblems(writeDenyFixture(t, adr, config)); len(problems) != 0 {
		t.Fatalf("complete prose-governed rules were rejected: %v", problems)
	}

	for _, test := range []struct {
		name, old, replacement, want string
	}{
		{"shared kernel loses a bounded context", sharedRule, rule("shared-kernel", "**/internal/shared/skillpkg/**", append(append([]string{}, bounded[:2]...), composition...)...), `rule "shared-kernel" does not deny "trace"`},
		{"shared kernel loses a composition root", sharedRule, rule("shared-kernel", "**/internal/shared/skillpkg/**", append(append([]string{}, bounded...), composition[1:]...)...), `rule "shared-kernel" does not deny "apiserver"`},
		{"generic loses worker", genericRule, rule("generic", "**/internal/foundation/observability/audit/**", "creator/workspace", "skill/discovery", "trial/evidence", "entrypoint/api/apiserver", "foundation/storage/objreconcile"), `rule "generic" does not deny "worker"`},
		{"objreconcile loses identity", objRule, rule("objreconcile", "**/internal/foundation/storage/objreconcile/**", "skill/discovery", "trial/evidence", "entrypoint/api/apiserver", "entrypoint/worker"), `rule "objreconcile" does not deny "identity"`},
		{"ordinary context loses worker", catalogRule, rule("catalog", "**/internal/skill/discovery/**", "entrypoint/api/apiserver", "foundation/storage/objreconcile"), `rule "catalog" does not deny "worker"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := strings.Replace(config, test.old, test.replacement, 1)
			problems := strings.Join(depguardDenyProblems(writeDenyFixture(t, adr, mutated)), "\n")
			if !strings.Contains(problems, test.want) {
				t.Fatalf("want %q after mutation, got %s", test.want, problems)
			}
		})
	}

	misplaced := strings.Replace(config,
		"            - \"**/internal/skill/discovery/**\"\n            - \"!$test\"",
		"            - \"**/internal/skill/discovery/**\"\n            - \"**/internal/foundation/observability/audit/**\"\n            - \"!$test\"", 1)
	if problems := strings.Join(depguardDenyProblems(writeDenyFixture(t, adr, misplaced)), "\n"); !strings.Contains(problems, `rule "catalog" unexpectedly selects "audit"`) {
		t.Fatalf("moving a Generic selector under a context rule was accepted: %s", problems)
	}
}

func TestDepguardDenyAllowsARuleToRefuseTheBlanketGrant(t *testing.T) {
	t.Parallel()

	for _, problem := range depguardDenyProblems(writeDenyFixture(t, denyADRTable, denyConfig())) {
		if strings.Contains(problem, "identity") {
			t.Fatalf("refusing the blanket identity grant was reported: %q", problem)
		}
	}
}

func TestDepguardSelectorsAcceptTerminalWildcardContextPaths(t *testing.T) {
	t.Parallel()
	declared := map[string]packageIdentity{
		"catalog":  {Kind: architectureCore, ID: "catalog", Path: "skill/*"},
		"skillpkg": {Kind: architectureSharedKernel, ID: "skillpkg", Path: "shared/skillpkg"},
		"contract": {Kind: architectureSharedKernel, ID: "contract", Path: "shared/contract"},
	}
	shared := strings.Replace(
		rule("shared-kernel", "**/internal/shared/skillpkg/**"),
		"            - \"!$test\"",
		"            - \"**/internal/shared/contract/**\"\n            - \"!$test\"", 1)
	rules := depguardRules("version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" +
		rule("catalog", "**/internal/skill/**") + shared)
	if problems := depguardSelectorProblems(rules, declared, "lint.yml"); len(problems) != 0 {
		t.Fatalf("terminal wildcard context path was rejected: %v", problems)
	}
}

func TestDepguardSelectorsRejectMalformedGlobs(t *testing.T) {
	t.Parallel()
	declared := map[string]packageIdentity{
		"catalog": {Kind: architectureCore, ID: "catalog", Path: "skill/discovery"},
	}
	for _, selector := range []string{"NO**/internal/skill/discovery/**", "**/internal/skill/discovery/**NO"} {
		rules := depguardRules("version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" + rule("catalog", selector))
		if problems := depguardSelectorProblems(rules, declared, "lint.yml"); len(problems) == 0 {
			t.Fatalf("malformed selector %q was accepted", selector)
		}
	}
}

func TestDepguardDenySaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("no appendix rows", func(t *testing.T) {
		adr := denyADRTable[:strings.Index(denyADRTable, "## 附錄 A")]
		problems := depguardDenyProblems(writeDenyFixture(t, adr, denyConfig()))
		if len(problems) == 0 {
			t.Fatal("an ADR with no appendix A rows was accepted")
		}
	})
	t.Run("no context rules", func(t *testing.T) {
		lint := "version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" +
			rule("generic", "**/internal/foundation/storage/objreconcile/**", "creator/workspace")
		problems := depguardDenyProblems(writeDenyFixture(t, denyADRTable, lint))
		if len(problems) == 0 {
			t.Fatal("a config with no Core/Supporting rule was accepted")
		}
	})
	t.Run("no config", func(t *testing.T) {
		root := writeDenyFixture(t, denyADRTable, denyConfig())
		if err := os.Remove(filepath.Join(root, "apps", "platform", ".golangci.yml")); err != nil {
			t.Fatal(err)
		}
		if problems := depguardDenyProblems(root); len(problems) == 0 {
			t.Fatal("a missing lint config was accepted")
		}
	})
}

func TestStripYAMLCommentsKeepsQuotedHashes(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		`  - "**/internal/skill/discovery/**"  # commented rule below`: `  - "**/internal/skill/discovery/**"  `,
		`# - "**/internal/skill/library/**"`:                           ``,
		`  desc: "see issue #123 for why"`:                             `  desc: "see issue #123 for why"`,
		`  desc: 'a # inside single quotes'`:                           `  desc: 'a # inside single quotes'`,
		`  plain: value`:                                               `  plain: value`,
	}
	for line, want := range tests {
		if got := stripYAMLComments(line); got != want {
			t.Errorf("stripYAMLComments(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestDepguardDenyIgnoresCommentedOutRules(t *testing.T) {
	t.Parallel()

	lint := strings.Replace(denyConfig(),
		"            - pkg: "+denyPrefix+"entrypoint/api/apiserver\n",
		"            # - pkg: "+denyPrefix+"entrypoint/api/apiserver\n", 1)
	problems := depguardDenyProblems(writeDenyFixture(t, denyADRTable, lint))
	if !strings.Contains(strings.Join(problems, "\n"), `does not deny "apiserver"`) {
		t.Fatalf("a commented-out deny entry still counted: %v", problems)
	}
}
