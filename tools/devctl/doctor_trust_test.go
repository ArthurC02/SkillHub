package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func trustConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWorkspaceTrustIsReportedForEverySpellingOfTheRepository(t *testing.T) {
	t.Parallel()
	const root = `C:\src\SkillHub`
	for _, tc := range []struct {
		name, config, status, detail string
	}{
		{"trusted under another case and slash", `{"projects":{"c:/src/skillhub/":{"hasTrustDialogAccepted":true}}}`, statusPass, "trusted"},
		{"only the old folder is trusted", `{"projects":{"C:/Users/a/OneDrive/SkillHub":{"hasTrustDialogAccepted":true}}}`, statusWarn, "has not trusted this folder"},
		{"one spelling left untrusted", `{"projects":{"C:/src/SkillHub":{"hasTrustDialogAccepted":true},"c:\\src\\SkillHub":{}}}`, statusWarn, `not trusted as c:\src\SkillHub:`},
		{"a sibling folder with the same prefix", `{"projects":{"C:/src/SkillHub2":{"hasTrustDialogAccepted":true}}}`, statusWarn, "has not trusted this folder"},
		{"unreadable config", `{"projects":`, statusWarn, "cannot read"},
	} {
		result := checkClaudeWorkspaceTrust(root, trustConfig(t, tc.config))
		if result.status != tc.status || !strings.Contains(result.detail, tc.detail) || result.required {
			t.Errorf("%s: got %s %q required=%v", tc.name, result.status, result.detail, result.required)
		}
	}
}

func TestWorkspaceTrustIsNotCheckedWithoutAClaudeCodeConfig(t *testing.T) {
	t.Parallel()
	result := checkClaudeWorkspaceTrust(`C:\src\SkillHub`, filepath.Join(t.TempDir(), ".claude.json"))
	if result.status != statusPass || !strings.Contains(result.detail, "nothing to check") {
		t.Fatalf("got %s %q", result.status, result.detail)
	}
}

func TestClaudeConfigFollowsClaudeConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if got := claudeConfigPath(); got != filepath.Join(dir, ".claude.json") {
		t.Fatalf("got %s", got)
	}
}
