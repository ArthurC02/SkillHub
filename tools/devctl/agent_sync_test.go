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

func TestAgentSyncLeavesAClaudeOnlyPluginOutOfThePortableSkills(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".claude/agents/writer.md", claudeAgentFixture)
	writeTestFile(t, root, ".claude/skills/guards/.claude-plugin/plugin.json", "{\"name\":\"guards\"}\n")
	writeTestFile(t, root, ".claude/skills/hybrid/.claude-plugin/plugin.json", "{\"name\":\"hybrid\"}\n")
	writeTestFile(t, root, ".claude/skills/hybrid/SKILL.md", "---\nname: hybrid\n---\n\nRoute.\n")

	if err := agentSync(root, nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/guards")); !os.IsNotExist(err) {
		t.Fatalf("a plugin with no SKILL.md reached .agents/skills: stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/hybrid/.claude-plugin/plugin.json")); err != nil {
		t.Fatalf("a skill that is also a plugin was not copied whole: %v", err)
	}
}

func TestParseClaudeAgentRejectsIncompleteFrontmatter(t *testing.T) {
	if _, err := parseClaudeAgent("---\nname: x\n---\n\nBody.\n"); err == nil {
		t.Fatal("incomplete frontmatter was accepted")
	}
}

func TestCopyTreeRefusesASymlink(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "out")
	real := filepath.Join(source, "real.txt")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(source, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create a symlink on this platform: %v", err)
	}
	err := copyTree(source, target)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("copyTree(%q) with a symlinked entry = %v, want an error mentioning symlink", source, err)
	}
}
