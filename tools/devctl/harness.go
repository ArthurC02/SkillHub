package main

// The agent harness under .claude/ has three rules that were prose until now.
//
// # Why this exists
//
// 2026-09-03 added roles (.claude/agents), generic skills (.claude/skills) and
// path-triggered pointers (.claude/rules), each with a placement rule written
// into AGENTS.md: a skill holds only what is true in any repository, so it must
// not cite docs/, an ADR or a requirement ID — cite one and the skill expires
// with that document; a role must name its model, because the standing rule
// of this repo is that subagents never inherit the session's model; and the
// root AGENTS.md is capped, because Codex stops reading project instructions at
// 32 KiB (`project_doc_max_bytes`) and the file was at 20.9 KiB and growing by
// one dated correction a day.
//
// None of that had a machine. The day after the rules were written, the review
// found two skills without a `name:` field — a defect nothing checked. This is
// the machine next to the sentence, in the shape every other checker here has.
//
// # What it does not check
//
// Whether a skill is actually generic; only that it names nothing local. Whether
// the model named is a good choice; only that one is named and it is not the
// values the rule forbids (fable, sol, inherit). Whether the rules' `paths:` globs match anything;
// that needs a session, not a file. Whether a skill's frontmatter is a valid
// Agent Skills manifest: the product's own validator does that, in
// apps/platform/internal/shared/skillpkg/repo_skills_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	harnessSkillsDir    = ".claude/skills"
	harnessAgentsDir    = ".claude/agents"
	harnessWorkflowsDir = ".claude/workflows"
	// Codex's project_doc_max_bytes default is 32 KiB and it truncates silently.
	// 16 KiB: the file was 22 KiB before 2026-09-04 and 12.9 KiB after the rewrite;
	// the cap is a ratchet, so it sits just above where the file is, not at the cliff.
	agentsDocMaxBytes = 16 * 1024
)

// A skill that cites any of these is bound to this repository.
var harnessLocalReferences = []*regexp.Regexp{
	regexp.MustCompile(`docs/`),
	requirementID, // requirement_refs.go: DISC-001, PORT-010, and ADR-034 — an ADR number has the same shape
}

// `model: opus` inside the frontmatter block.
var agentModelLine = regexp.MustCompile(`(?m)^model:\s*(\S+)\s*$`)

func harnessProblems(root string) []string {
	var problems []string
	problems = append(problems, harnessSkillProblems(root)...)
	problems = append(problems, harnessAgentProblems(root)...)
	problems = append(problems, harnessAgentsDocProblems(root)...)
	problems = append(problems, harnessWorkflowProblems(root)...)
	return problems
}

func harnessSkillProblems(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(harnessSkillsDir), "*", "SKILL.md"))
	sort.Strings(matches)
	var problems []string
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("harness: %v", err))
			continue
		}
		relative := harnessRelative(root, path)
		for i, line := range strings.Split(string(data), "\n") {
			for _, pattern := range harnessLocalReferences {
				if hit := pattern.FindString(line); hit != "" {
					problems = append(problems, fmt.Sprintf(
						"harness: %s:%d cites %q; a skill holds only what is true in any repository, "+
							"so the rule this line depends on belongs in docs/ or an AGENTS.md and the skill "+
							"keeps the generic procedure (AGENTS.md 分區指標與攔阻)",
						relative, i+1, hit))
				}
			}
		}
	}
	return problems
}

func harnessAgentProblems(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(harnessAgentsDir), "*.md"))
	sort.Strings(matches)
	var problems []string
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("harness: %v", err))
			continue
		}
		relative := harnessRelative(root, path)
		frontmatter, ok := frontmatterOf(string(data))
		if !ok {
			problems = append(problems, fmt.Sprintf("harness: %s has no frontmatter block", relative))
			continue
		}
		m := agentModelLine.FindStringSubmatch(frontmatter)
		switch {
		case m == nil:
			problems = append(problems, fmt.Sprintf(
				"harness: %s names no `model:`; a role that inherits the session's model is the one "+
					"thing this repo's subagent rule forbids", relative))
		case strings.EqualFold(m[1], "inherit"), strings.Contains(strings.ToLower(m[1]), "fable"), strings.Contains(strings.ToLower(m[1]), "sol"):
			problems = append(problems, fmt.Sprintf(
				"harness: %s sets `model: %s`; subagents must name a model and it must not be fable, sol "+
					"or inherit", relative, m[1]))
		}
	}
	return problems
}

func harnessAgentsDocProblems(root string) []string {
	info, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return []string{fmt.Sprintf("harness: %v", err)}
	}
	if info.Size() <= agentsDocMaxBytes {
		return nil
	}
	return []string{fmt.Sprintf(
		"harness: AGENTS.md is %d bytes, over the %d-byte cap. Codex stops reading project instructions "+
			"at 32 KiB and says nothing; move the growth into the directory's AGENTS.md or docs/ and leave "+
			"a pointer (AGENTS.md 分區指標與攔阻)", info.Size(), agentsDocMaxBytes)}
}

func harnessRelative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// The text between the opening `---` line and the next `---` line.
func frontmatterOf(text string) (string, bool) {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", false
	}
	rest := text[len("---\n"):]
	if i := strings.Index(rest, "\n---\n"); i >= 0 {
		return rest[:i], true
	}
	if strings.HasPrefix(rest, "---\n") {
		return "", true
	}
	return "", false
}

// A workflow script's agent() inherits the dispatcher's model unless told
// otherwise, and the dispatcher's model is the flagship tier the subagent rule
// forbids. So every agent( call must sit on a line that also names model:, and
// no model literal may be fable, sol or inherit. Scripts get there with a
// one-line wrapper — const run = (p, o = {}) => agent(p, { ...o, model: o.model ?? 'sonnet' })
// — which is why the check is per line, not per call: a call split across lines
// is flagged, and the fix is to route it through the wrapper.
var (
	workflowAgentCall     = regexp.MustCompile(`\bagent\(`)
	workflowModelLiteral  = regexp.MustCompile(`model:\s*['"]([^'"]*)['"]`)
	workflowMetaName      = regexp.MustCompile(`(?s)^\s*export const meta = \{.*?name:\s*['"]([^'"]+)['"]`)
	workflowForbiddenName = regexp.MustCompile(`(?i)fable|sol|inherit`)
)

func harnessWorkflowProblems(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(harnessWorkflowsDir), "*.js"))
	sort.Strings(matches)
	var problems []string
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("harness: %v", err))
			continue
		}
		relative := harnessRelative(root, path)
		text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\uFEFF"), "\r\n", "\n")
		stem := strings.TrimSuffix(filepath.Base(path), ".js")
		if m := workflowMetaName.FindStringSubmatch(text); m == nil {
			problems = append(problems, fmt.Sprintf(
				"harness: %s does not begin with `export const meta = { name: ... }`; Claude Code drops "+
					"the /%s command silently when it does not", relative, stem))
		} else if m[1] != stem {
			problems = append(problems, fmt.Sprintf(
				"harness: %s declares meta.name %q; the file name is the command name, keep them equal",
				relative, m[1]))
		}
		for i, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue // prose about agent() is not a call
			}
			if workflowAgentCall.MatchString(line) && !strings.Contains(line, "model:") {
				problems = append(problems, fmt.Sprintf(
					"harness: %s:%d calls agent() without model: on the same line; a bare agent() inherits "+
						"the dispatcher's flagship model, which subagents may not use (AGENTS.md 開發自動化 3)",
					relative, i+1))
			}
			for _, m := range workflowModelLiteral.FindAllStringSubmatch(line, -1) {
				if workflowForbiddenName.MatchString(m[1]) {
					problems = append(problems, fmt.Sprintf(
						"harness: %s:%d sets model %q; subagents must not use fable, sol or inherit",
						relative, i+1, m[1]))
				}
			}
		}
	}
	return problems
}
