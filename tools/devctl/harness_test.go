package main

import (
	"strings"
	"testing"
)

func TestTheRealHarnessKeepsItsOwnRules(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := harnessProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
}

const cleanSkill = "---\nname: x\ndescription: generic\n---\n\nRun the one test, watch it go red.\n"
const cleanAgent = "---\nname: x\ndescription: y\nmodel: opus\neffort: high\n---\n\nBody.\n"

func writeHarnessFixture(t *testing.T, skill, agent, agents string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, harnessSkillsDir+"/x/SKILL.md", skill)
	writeAt(t, root, harnessAgentsDir+"/x.md", agent)
	writeAt(t, root, "AGENTS.md", agents)
	return root
}

func TestHarnessAcceptsAGenericSkillAndANamedModel(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a clean harness was rejected: %v", problems)
	}
}

func TestHarnessRejectsASkillBoundToThisRepo(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"ADR number":     "Decided in " + adrNumber(34) + ".",
		"requirement id": "This satisfies PORT-004.",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeHarnessFixture(t, cleanSkill+line+"\n", cleanAgent, "# 導覽\n")
			problems := harnessProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], ".claude/skills/x/SKILL.md:7 cites") {
				t.Fatalf("a skill citing a local document was accepted: %v", problems)
			}
		})
	}
}

func TestHarnessRejectsASkillDirectoryWithNeitherSkillNorPlugin(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	writeAt(t, root, harnessSkillsDir+"/stray/README.md", "notes\n")
	problems := harnessProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], ".claude/skills/stray has neither SKILL.md") {
		t.Fatalf("a skill directory with neither SKILL.md nor a plugin manifest was accepted: %v", problems)
	}
}

func TestHarnessAcceptsAClaudeOnlyPluginBesideTheSkills(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	writeAt(t, root, harnessSkillsDir+"/guards/.claude-plugin/plugin.json", "{\"name\":\"guards\"}\n")
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a plugin directory under .claude/skills was rejected: %v", problems)
	}
}

func TestHarnessAcceptsASkillThatNamesADocumentationFolder(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill+"Do not default to `docs/` or any other location.\n", cleanAgent, "# 導覽\n")
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a skill naming a documentation folder was rejected: %v", problems)
	}
}

func TestHarnessRejectsARoleWithoutAModel(t *testing.T) {
	t.Parallel()
	for name, agent := range map[string]string{
		"absent":  "---\nname: x\ndescription: y\neffort: low\n---\n\nBody.\n",
		"inherit": "---\nname: x\nmodel: inherit\neffort: low\n---\n\nBody.\n",
		"fable":   "---\nname: x\nmodel: claude-fable-5-1\neffort: low\n---\n\nBody.\n",
		"sol":     "---\nname: x\nmodel: sol\neffort: low\n---\n\nBody.\n",

		"in the body": "---\nname: x\neffort: low\n---\n\nmodel: opus\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeHarnessFixture(t, cleanSkill, agent, "# 導覽\n")
			problems := harnessProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], ".claude/agents/x.md") {
				t.Fatalf("a role without a usable model was accepted: %v", problems)
			}
		})
	}
}

func TestHarnessRejectsARoleWithoutAUsableEffort(t *testing.T) {
	t.Parallel()
	for name, agent := range map[string]string{
		"absent":      "---\nname: x\nmodel: sonnet\n---\n\nBody.\n",
		"unknown":     "---\nname: x\nmodel: sonnet\neffort: extreme\n---\n\nBody.\n",
		"in the body": "---\nname: x\nmodel: sonnet\n---\n\neffort: low\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeHarnessFixture(t, cleanSkill, agent, "# 導覽\n")
			problems := harnessProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], ".claude/agents/x.md") || !strings.Contains(problems[0], "effort") {
				t.Fatalf("a role without a usable effort was accepted: %v", problems)
			}
		})
	}
}

func TestHarnessAcceptsEveryEffortLevel(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			agent := "---\nname: x\nmodel: sonnet\neffort: " + level + "\n---\n\nBody.\n"
			root := writeHarnessFixture(t, cleanSkill, agent, "# 導覽\n")
			if problems := harnessProblems(root); len(problems) != 0 {
				t.Fatalf("effort %s was rejected: %v", level, problems)
			}
		})
	}
}

func TestHarnessRejectsARootAgentsDocOverTheCap(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, strings.Repeat("x", agentsDocMaxBytes+1))
	problems := harnessProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "over the 10240-byte cap") {
		t.Fatalf("an AGENTS.md over the Codex cap was accepted: %v", problems)
	}
}

const cleanWorkflow = "export const meta = {\n  name: 'x',\n  description: 'y',\n}\nconst run = (p, o = {}) => agent(p, { ...o, model: o.model ?? 'sonnet', effort: o.effort ?? 'medium' })\nawait run('do it', { label: 'a' })\n"

func TestHarnessAcceptsAWorkflowThatNamesItsModels(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	writeAt(t, root, harnessWorkflowsDir+"/x.js", cleanWorkflow)
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a clean workflow was rejected: %v", problems)
	}
}

func TestHarnessRejectsAWorkflowThatInheritsTheDispatcherModel(t *testing.T) {
	t.Parallel()
	for name, script := range map[string]string{
		"no model":      cleanWorkflow + "await agent('again', { effort: 'low' })\n",
		"no effort":     cleanWorkflow + "await agent('again', { model: 'sonnet' })\n",
		"agent split":   cleanWorkflow + "await agent('again', { effort: 'low',\n  model: 'sonnet' },\n)\n",
		"fable literal": cleanWorkflow + "await run('again', { model: 'claude-fable-5-1' })\n",
		"sol literal":   cleanWorkflow + "await run('again', { model: 'sol' })\n",
		"inherit":       cleanWorkflow + "await run('again', { model: 'inherit' })\n",
		"no meta":       "const run = (p, o = {}) => agent(p, { ...o, model: 'sonnet', effort: 'low' })\n",
		"name mismatch": strings.Replace(cleanWorkflow, "name: 'x'", "name: 'y'", 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
			writeAt(t, root, harnessWorkflowsDir+"/x.js", script)
			problems := harnessProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], ".claude/workflows/x.js") {
				t.Fatalf("a workflow that can reach the dispatcher's model was accepted: %v", problems)
			}
		})
	}
}

func TestHarnessIgnoresProseAboutAgentCalls(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")

	writeAt(t, root, harnessWorkflowsDir+"/x.js", cleanWorkflow+"// a bare agent() would inherit\n")
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a comment mentioning agent() was treated as a call: %v", problems)
	}
}

func TestHarnessRejectsARoleThatPreloadsAMissingSkill(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		skills  string
		missing bool
	}{
		"an existing skill":           {"skills: [x]\n", false},
		"no skills line":              {"", false},
		"an empty list":               {"skills: []\n", false},
		"a deleted skill":             {"skills: [gone]\n", true},
		"one present and one deleted": {"skills: [x, gone]\n", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			agent := "---\nname: x\ndescription: y\nmodel: opus\neffort: high\n" + c.skills + "---\n\nBody.\n"
			problems := harnessProblems(writeHarnessFixture(t, cleanSkill, agent, "# 導覽\n"))
			named := len(problems) == 1 && strings.Contains(problems[0], `preloads skill "gone"`)
			if named != c.missing || (!c.missing && len(problems) != 0) {
				t.Fatalf("problems = %v, want a missing-skill problem: %v", problems, c.missing)
			}
		})
	}
}

func TestHarnessAcceptsADenyRulePairedAcrossBothShells(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	writeAt(t, root, harnessSettingsFile,
		`{"permissions":{"deny":["Bash(*git stash*)","PowerShell(*git stash*)","Edit(secrets/**)"]}}`)
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a deny rule present in both shells was rejected: %v", problems)
	}
}

func TestHarnessRejectsADenyRuleMissingFromOneShell(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ present, missing string }{
		{"Bash", "PowerShell"},
		{"PowerShell", "Bash"},
	} {
		t.Run("only "+tc.present, func(t *testing.T) {
			t.Parallel()
			root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
			writeAt(t, root, harnessSettingsFile,
				`{"permissions":{"deny":["Bash(*git stash*)","PowerShell(*git stash*)","`+tc.present+`(*git clean -*)"]}}`)
			problems := harnessProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.missing+"(*git clean -*)") {
				t.Fatalf("a deny rule only %s carries was accepted: %v", tc.present, problems)
			}
		})
	}
}

func TestHarnessRejectsUnreadableSettings(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, "# 導覽\n")
	writeAt(t, root, harnessSettingsFile, `{"permissions":`)
	if problems := harnessProblems(root); len(problems) != 1 || !strings.Contains(problems[0], harnessSettingsFile) {
		t.Fatalf("settings that do not parse were accepted: %v", problems)
	}
}

func TestHarnessAcceptsARootAgentsDocExactlyAtTheCap(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, strings.Repeat("x", agentsDocMaxBytes))
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("an AGENTS.md exactly at the cap was refused: %v", problems)
	}
}
