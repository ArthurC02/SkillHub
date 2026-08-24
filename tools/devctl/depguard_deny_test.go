package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Three contexts plus the two non-context packages every rule must deny. The
// kinds matter: only Core and Supporting rows join the universe a rule chooses
// from, and only a rule guarding one of them is reconciled at all.
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

// rule renders one depguard rule at the indentation the real file uses.
func rule(name, selector string, deny ...string) string {
	out := "        " + name + ":\n          files:\n            - \"" + selector + "\"\n" +
		"            - \"!$test\"\n          deny:\n"
	for _, pkg := range deny {
		out += "            - pkg: " + denyPrefix + pkg + "\n" +
			"              desc: \"cross-context import forbidden by ADR-032\"\n"
	}
	return out
}

// A config in which every context denies everything appendix A does not keep.
// catalog keeps trace; everyone keeps identity through the blanket row; policy's
// real-world refusal of that blanket row is modelled by trace, which denies
// identity anyway.
func denyConfig() string {
	return "version: \"2\"\nlinters:\n  settings:\n    depguard:\n      rules:\n" +
		rule("identity", "**/internal/creator/workspace/**",
			"skill/discovery", "trial/evidence", "entrypoint/api/apiserver", "foundation/storage/objreconcile") +
		rule("catalog", "**/internal/skill/discovery/**",
			"entrypoint/api/apiserver", "foundation/storage/objreconcile") +
		rule("trace", "**/internal/trial/evidence/**",
			"creator/workspace", "skill/discovery", "entrypoint/api/apiserver", "foundation/storage/objreconcile")
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
		// THE mutation. Deleting the `- pkg:`/`desc:` pair from identity's rule
		// hands creator/workspace a free import of skill/discovery, adds and
		// removes no drift marker, leaves a rule in place for the path so
		// contextMapProblems is happy, and does not touch appendix A.
		name: "a deny entry was deleted, which is how a permission is granted here",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			"            - pkg: "+denyPrefix+"skill/discovery\n"+
				"              desc: \"cross-context import forbidden by ADR-032\"\n", "", 1),
		want: `rule "identity" does not deny "catalog"`,
	}, {
		// The composition root is not a context and does not come from §1's
		// Core/Supporting rows, so it needs naming or its deny line is free to go.
		name: "the apiserver deny line was deleted",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			"            - pkg: "+denyPrefix+"entrypoint/api/apiserver\n"+
				"              desc: \"cross-context import forbidden by ADR-032\"\n", "", 1),
		want: `rule "identity" does not deny "apiserver"`,
	}, {
		// The other direction: the lint refuses a collaboration appendix A keeps.
		// Not a hole, but the two sides now disagree about a named pair, which is
		// exactly what the config's header promises cannot happen.
		name: "the lint denies a pair the appendix keeps",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(),
			rule("catalog", "**/internal/skill/discovery/**",
				"entrypoint/api/apiserver", "foundation/storage/objreconcile"),
			rule("catalog", "**/internal/skill/discovery/**",
				"trial/evidence", "entrypoint/api/apiserver", "foundation/storage/objreconcile"), 1),
		want: "appendix A keeps `catalog` → `trace`",
	}, {
		// A permission removed from the appendix must be denied again. Appendix A
		// says removed rows are not added back without a new ADR; until the rule
		// follows, the import is legal in the only place that enforces anything.
		name: "the appendix row was removed and the rule still allows it",
		adr:  strings.Replace(denyADRTable, "| 同步查詢，合法 | 保留（DDD-004） |", "| 事件化 | 移出 |", 1),
		lint: denyConfig(),
		want: `rule "catalog" does not deny "trace"`,
	}, {
		// A typo in a pkg path is a deny entry that denies nothing.
		name: "a deny entry names a package no §1 row declares",
		adr:  denyADRTable,
		lint: strings.Replace(denyConfig(), denyPrefix+"skill/discovery", denyPrefix+"skill/discovry", 1),
		want: "denies internal/skill/discovry, which no",
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

// A rule refusing a BLANKET grant is stricter than the appendix and cannot open
// a hole: the real `policy` rule denies identity on purpose. If this were
// reported, the check's first act on the real tree would be to ask somebody to
// weaken a rule.
func TestDepguardDenyAllowsARuleToRefuseTheBlanketGrant(t *testing.T) {
	t.Parallel()
	// trace already denies identity in the fixture; assert that is not a problem.
	for _, problem := range depguardDenyProblems(writeDenyFixture(t, denyADRTable, denyConfig())) {
		if strings.Contains(problem, "identity") {
			t.Fatalf("refusing the blanket identity grant was reported: %q", problem)
		}
	}
}

// Both inputs can lose their subject: an appendix with no rows, and a config
// with no context rule left. Either one silently passes a naive implementation.
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

// FIX 7. depguardFilePattern runs over text, so a rule commented out during
// debugging - or any `**/internal/x/y/**` written in prose - used to register the
// path as guarded, in this check and in contextMapProblems.
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
	// catalog's deny of apiserver is commented out. It must count as absent.
	lint := strings.Replace(denyConfig(),
		"            - pkg: "+denyPrefix+"entrypoint/api/apiserver\n",
		"            # - pkg: "+denyPrefix+"entrypoint/api/apiserver\n", 1)
	problems := depguardDenyProblems(writeDenyFixture(t, denyADRTable, lint))
	if !strings.Contains(strings.Join(problems, "\n"), `does not deny "apiserver"`) {
		t.Fatalf("a commented-out deny entry still counted: %v", problems)
	}
}
