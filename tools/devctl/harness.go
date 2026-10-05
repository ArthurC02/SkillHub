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

const (
	harnessSkillsDir    = ".claude/skills"
	harnessAgentsDir    = ".claude/agents"
	harnessWorkflowsDir = ".claude/workflows"

	agentsDocMaxBytes = 16 * 1024
)

var harnessLocalReferences = []*regexp.Regexp{
	requirementID,
}

// Matches "model: opus" inside a frontmatter block.
var agentModelLine = regexp.MustCompile(`(?m)^model:\s*(\S+)\s*$`)

var (
	agentEffortLine = regexp.MustCompile(`(?m)^effort:\s*(\S+)\s*$`)
	effortLevels    = strings.Fields("low medium high xhigh max")
)

func harnessProblems(root string) []string {
	var problems []string
	problems = append(problems, harnessSkillDirectoryProblems(root)...)
	problems = append(problems, harnessSkillProblems(root)...)
	problems = append(problems, harnessAgentProblems(root)...)
	problems = append(problems, harnessAgentsDocProblems(root)...)
	problems = append(problems, harnessWorkflowProblems(root)...)
	return problems
}

func harnessSkillDirectoryProblems(root string) []string {
	skills := filepath.Join(root, filepath.FromSlash(harnessSkillsDir))
	entries, _ := os.ReadDir(skills)
	var problems []string
	for _, entry := range entries {
		dir := filepath.Join(skills, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); !entry.IsDir() || err == nil || isClaudeOnlyPlugin(dir) {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"harness: %s has neither SKILL.md nor .claude-plugin/plugin.json; a skill directory holds one of them "+
				"(docs/development/automation.md Harness)", harnessRelative(root, dir)))
	}
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
		problems = append(problems, roleEffortProblems(relative, frontmatter)...)
	}
	return problems
}

func roleEffortProblems(relative, frontmatter string) []string {
	m := agentEffortLine.FindStringSubmatch(frontmatter)
	switch {
	case m == nil:
		return []string{fmt.Sprintf(
			"harness: %s names no `effort:`; a role without one runs at whatever effort the dispatching "+
				"session uses, so the tier it was chosen for decides nothing (docs/development/automation.md "+
				"共享工作樹與 SubAgent)", relative)}
	case !slices.Contains(effortLevels, m[1]):
		return []string{fmt.Sprintf(
			"harness: %s sets `effort: %s`; use one of %s", relative, m[1], strings.Join(effortLevels, ", "))}
	}
	return nil
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

// A call is flagged only when its own line lacks "model:", so a call split
// across multiple lines must be rewritten onto one.
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
		problems = append(problems, workflowAgentCallProblems(relative, text)...)
	}
	return problems
}

func workflowAgentCallProblems(relative, text string) []string {
	var problems []string
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue // a comment line is prose, not a call
		}
		if workflowAgentCall.MatchString(line) && !strings.Contains(line, "model:") {
			problems = append(problems, fmt.Sprintf(
				"harness: %s:%d calls agent() without model: on the same line; a bare agent() inherits "+
					"the dispatcher's flagship model, which subagents may not use (AGENTS.md 開發自動化 3)",
				relative, i+1))
		}
		if workflowAgentCall.MatchString(line) && !strings.Contains(line, "effort:") {
			problems = append(problems, fmt.Sprintf(
				"harness: %s:%d calls agent() without effort: on the same line; the agent then runs at the "+
					"dispatching session's effort instead of the one its tier calls for",
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
	return problems
}
