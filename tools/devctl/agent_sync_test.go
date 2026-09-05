package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const claudeAgentFixture = "---\nname: writer\ndescription: writes files\nmodel: sonnet\nskills: [false-green]\n---\n\nOnly write approved paths.\n"

func TestAgentSyncGeneratesPortableArtifactsAndDetectsDrift(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".claude/agents/writer.md", claudeAgentFixture)
	writeTestFile(t, root, ".claude/skills/false-green/SKILL.md", "---\nname: false-green\n---\n\nWatch the exit code.\n")

	var out bytes.Buffer
	if err := agentSync(root, nil, &out); err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(filepath.Join(root, ".agents/skills/false-green/SKILL.md"))
	if err != nil || !strings.Contains(string(skill), "Watch the exit code.") {
		t.Fatalf("generated skill = %q, %v", skill, err)
	}
	codex, err := os.ReadFile(filepath.Join(root, ".codex/agents/writer.toml"))
	if err != nil || !strings.Contains(string(codex), "developer_instructions = \"Only write approved paths.\\n\"") {
		t.Fatalf("generated Codex agent = %q, %v", codex, err)
	}
	if err := agentSync(root, []string{"--check"}, &out); err != nil {
		t.Fatalf("current output rejected: %v", err)
	}

	writeTestFile(t, root, ".codex/agents/writer.toml", "hand edit\n")
	out.Reset()
	if err := agentSync(root, []string{"--check"}, &out); err == nil || !strings.Contains(out.String(), "DRIFT Codex agents writer.toml") {
		t.Fatalf("hand edit was accepted: err=%v output=%q", err, out.String())
	}
}

func TestParseClaudeAgentRejectsIncompleteFrontmatter(t *testing.T) {
	if _, _, _, err := parseClaudeAgent("---\nname: x\n---\n\nBody.\n"); err == nil {
		t.Fatal("incomplete frontmatter was accepted")
	}
}
