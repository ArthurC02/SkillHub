package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateGeneratedContentRejectsTheRepoAbsolutePath(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	generated := filepath.Join(root, "generated")
	writeTestFile(t, generated, "models.go", "// from "+repoRoot+"\npackage models\n")
	err := validateGeneratedContent(generated, repoRoot)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("validateGeneratedContent() = %v, want an absolute-path rejection", err)
	}
}

func TestValidateGeneratedContentRejectsATimestampHeaderInTheFirst20Lines(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	generated := filepath.Join(root, "generated")
	writeTestFile(t, generated, "models.go", "// Generated at: 2026-09-11\npackage models\n")
	err := validateGeneratedContent(generated, repoRoot)
	if err == nil || !strings.Contains(err.Error(), "timestamp") {
		t.Fatalf("validateGeneratedContent() = %v, want a timestamp-header rejection", err)
	}
}

func TestValidateGeneratedContentAcceptsATimestampMentionPastTheHeaderWindow(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	generated := filepath.Join(root, "generated")
	lines := make([]string, 0, 21)
	for i := 0; i < 20; i++ {
		lines = append(lines, "package models")
	}
	lines = append(lines, "// Generated at: 2026-09-11")
	writeTestFile(t, generated, "models.go", strings.Join(lines, "\n")+"\n")
	if err := validateGeneratedContent(generated, repoRoot); err != nil {
		t.Fatalf("a timestamp mention on line 21 was rejected: %v", err)
	}
}

func TestValidateGeneratedContentAcceptsACleanFile(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	generated := filepath.Join(root, "generated")
	writeTestFile(t, generated, "models.go", "package models\n")
	if err := validateGeneratedContent(generated, repoRoot); err != nil {
		t.Fatalf("a clean file was rejected: %v", err)
	}
}
