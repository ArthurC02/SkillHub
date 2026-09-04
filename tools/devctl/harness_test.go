package main

import (
	"strings"
	"testing"
)

// Pointed at the tree. RED the day a skill cites a document, a role drops its
// model, or the root AGENTS.md crosses the cap.
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
const cleanAgent = "---\nname: x\ndescription: y\nmodel: opus\n---\n\nBody.\n"

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
		"docs path":      "See docs/development/automation.md first.",
		"ADR number":     "Decided in ADR-034.",
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

func TestHarnessRejectsARoleWithoutAModel(t *testing.T) {
	t.Parallel()
	for name, agent := range map[string]string{
		"absent":  "---\nname: x\ndescription: y\n---\n\nBody.\n",
		"inherit": "---\nname: x\nmodel: inherit\n---\n\nBody.\n",
		"fable":   "---\nname: x\nmodel: claude-fable-5-1\n---\n\nBody.\n",
		"sol":     "---\nname: x\nmodel: sol\n---\n\nBody.\n",
		// A `model:` in the body is prose, not frontmatter.
		"in the body": "---\nname: x\n---\n\nmodel: opus\n",
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

func TestHarnessRejectsARootAgentsDocOverTheCap(t *testing.T) {
	t.Parallel()
	root := writeHarnessFixture(t, cleanSkill, cleanAgent, strings.Repeat("x", agentsDocMaxBytes+1))
	problems := harnessProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "over the 16384-byte cap") {
		t.Fatalf("an AGENTS.md over the Codex cap was accepted: %v", problems)
	}
}

const cleanWorkflow = "export const meta = {\n  name: 'x',\n  description: 'y',\n}\nconst run = (p, o = {}) => agent(p, { ...o, model: o.model ?? 'sonnet' })\nawait run('do it', { label: 'a' })\n"

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
		"bare agent()":  cleanWorkflow + "await agent('again')\n",
		"agent split":   cleanWorkflow + "await agent(\n  'again',\n  { model: 'sonnet' },\n)\n",
		"fable literal": cleanWorkflow + "await run('again', { model: 'claude-fable-5-1' })\n",
		"sol literal":   cleanWorkflow + "await run('again', { model: 'sol' })\n",
		"inherit":       cleanWorkflow + "await run('again', { model: 'inherit' })\n",
		"no meta":       "const run = (p, o = {}) => agent(p, { ...o, model: 'sonnet' })\n",
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
	// After meta, not before: meta must be the file's first statement.
	writeAt(t, root, harnessWorkflowsDir+"/x.js", cleanWorkflow+"// a bare agent() would inherit\n")
	if problems := harnessProblems(root); len(problems) != 0 {
		t.Fatalf("a comment mentioning agent() was treated as a call: %v", problems)
	}
}
