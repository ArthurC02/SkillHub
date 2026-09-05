package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// agentSync makes .claude/ the single source of truth for portable agent
// artifacts. The generated trees stay committed so each agent can use them
// locally; --check is the CI ratchet against hand edits and missed syncs.
func agentSync(root string, args []string, out io.Writer) error {
	check := false
	for _, arg := range args {
		if arg != "--check" {
			return fmt.Errorf("unknown agent-sync option %q", arg)
		}
		check = true
	}

	scratchRoot := filepath.Join(root, ".devctl")
	if err := os.MkdirAll(scratchRoot, 0o755); err != nil {
		return err
	}
	scratch, err := os.MkdirTemp(scratchRoot, "agent-sync-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	skillsSource := filepath.Join(root, ".claude", "skills")
	skillsOutput := filepath.Join(scratch, ".agents", "skills")
	if err := copyTree(skillsSource, skillsOutput); err != nil {
		return fmt.Errorf("copy Claude skills: %w", err)
	}
	codexOutput := filepath.Join(scratch, ".codex", "agents")
	if err := writeCodexAgents(filepath.Join(root, ".claude", "agents"), codexOutput); err != nil {
		return err
	}

	outputs := []generationOutput{
		{label: "agent skills", source: skillsOutput, target: filepath.Join(root, ".agents", "skills")},
		{label: "Codex agents", source: codexOutput, target: filepath.Join(root, ".codex", "agents")},
	}
	for _, output := range outputs {
		drift, err := compareTrees(output.source, output.target)
		if err != nil {
			return err
		}
		if len(drift) == 0 {
			fmt.Fprintf(out, "%s: generated output is current\n", output.label)
			continue
		}
		if check {
			for _, path := range drift {
				fmt.Fprintln(out, "DRIFT", output.label, filepath.ToSlash(path))
			}
			return errors.New("agent output is stale; run task agents:sync and commit the result")
		}
		if err := atomicReplaceDir(output.source, output.target); err != nil {
			return fmt.Errorf("replace %s: %w", output.label, err)
		}
		fmt.Fprintf(out, "%s: updated\n", output.label)
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		output := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(output, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to copy symlink %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(output, data, 0o644)
	})
}

func writeCodexAgents(source, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, entry := range sortedMarkdownFiles(entries) {
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return err
		}
		name, description, body, err := parseClaudeAgent(string(data))
		if err != nil {
			return fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		output := "# Code generated from .claude/agents/" + entry.Name() + "; DO NOT EDIT.\n" +
			"name = " + strconv.Quote(name) + "\n" +
			"description = " + strconv.Quote(description) + "\n" +
			"developer_instructions = " + strconv.Quote(body) + "\n"
		if err := os.WriteFile(filepath.Join(target, strings.TrimSuffix(entry.Name(), ".md")+".toml"), []byte(output), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func parseClaudeAgent(source string) (name, description, body string, err error) {
	source = strings.TrimPrefix(strings.ReplaceAll(source, "\r\n", "\n"), "\uFEFF")
	if !strings.HasPrefix(source, "---\n") {
		return "", "", "", errors.New("missing frontmatter")
	}
	end := strings.Index(source[len("---\n"):], "\n---\n")
	if end < 0 {
		return "", "", "", errors.New("unterminated frontmatter")
	}
	frontmatterEnd := len("---\n") + end
	fields := map[string]string{}
	for _, line := range strings.Split(source[len("---\n"):frontmatterEnd], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if fields["name"] == "" || fields["description"] == "" {
		return "", "", "", errors.New("frontmatter needs name and description")
	}
	body = strings.TrimSpace(source[frontmatterEnd+len("\n---\n"):]) + "\n"
	return fields["name"], fields["description"], body, nil
}

// Sorted so an unexpected filesystem order cannot make generated diffs noisy.
func sortedMarkdownFiles(entries []os.DirEntry) []os.DirEntry {
	var markdown []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			markdown = append(markdown, entry)
		}
	}
	sort.Slice(markdown, func(i, j int) bool { return markdown[i].Name() < markdown[j].Name() })
	return markdown
}
