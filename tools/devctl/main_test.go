package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibleVersionMatchesMajorMinor(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		got, want string
		match     bool
	}{
		{"go version go1.25.7 linux/amd64", "go1.25.0", true},
		{"v22.18.0", "v22.14.0", false},
		{"Python 3.12.9", "Python 3.12", true},
		{"go version go1.26.0 windows/amd64", "go1.25.0", false},
		{"v25.0.0", "v22.14.0", false},
	} {
		if got := compatibleVersion(test.got, test.want); got != test.match {
			t.Errorf("compatibleVersion(%q, %q) = %v; want %v", test.got, test.want, got, test.match)
		}
	}
}

func TestParseToolchain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "toolchain.yaml")
	contents := "schema_version: 1\n\ntools:\n  task: \"3.45.4\"\n  sqlc: '1.31.1'\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := parseToolchain(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["task"] != "3.45.4" || values["sqlc"] != "1.31.1" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestEnvInitDoesNotOverwriteExistingFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("SAFE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := envInit(root, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SECRET=kept\n" {
		t.Fatalf("existing .env was overwritten: %q", data)
	}
}

func TestEnvInitCopiesTemplate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("SECRET=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := envInit(root, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SECRET=\n" {
		t.Fatalf("unexpected .env contents: %q", data)
	}
}

func TestProfileCheckNamesMissingVariablesWithoutValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("LITELLM_MASTER_KEY", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("OPENAI_API_KEY=\nLITELLM_MASTER_KEY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	err := profileCheck(root, "model", &output)
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") || !strings.Contains(err.Error(), "LITELLM_MASTER_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileCheckAcceptsEnvironmentOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret-one")
	t.Setenv("LITELLM_MASTER_KEY", "secret-two")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := profileCheck(root, "model", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "secret-one") || strings.Contains(output.String(), "secret-two") {
		t.Fatal("profile check exposed secret values")
	}
}
