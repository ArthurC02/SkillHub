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

func TestParseManifestSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "toolchain.yaml")
	contents := "tools:\n  sqlc: \"1.31.1\"\nimages:\n  openapi_generator: \"image@sha256:abc\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := parseManifestSection(path, "images")
	if err != nil {
		t.Fatal(err)
	}
	if values["openapi_generator"] != "image@sha256:abc" {
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

func TestCheckPgliteInstallReconcilesAgainstToolchainPin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePackageJSON := func(module, version string) {
		dir := filepath.Join(root, "tools", "pglite", "node_modules", module)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := []byte(`{"name":"` + module + `","version":"` + version + `"}`)
		if err := os.WriteFile(filepath.Join(dir, "package.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writePackageJSON("@electric-sql/pglite", "0.4.6")
	writePackageJSON("@electric-sql/pglite-socket", "0.1.6")

	toolchain := map[string]string{"pglite": "0.4.6", "pglite_socket": "0.1.6"}
	results := checkPgliteInstall(root, toolchain)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %#v", len(results), results)
	}
	for _, r := range results {
		if r.status != "PASS" {
			t.Errorf("%s: expected PASS, got %s (%s)", r.name, r.status, r.detail)
		}
	}

	// Mutation: drift the pin away from what's actually installed. This
	// must FAIL, not warn or silently pass -- a version mismatch is an
	// environment diagnosis, not something to skip past (02:PORT-001).
	drifted := map[string]string{"pglite": "9.9.9", "pglite_socket": "0.1.6"}
	driftedResults := checkPgliteInstall(root, drifted)
	if driftedResults[0].status != "FAIL" || !driftedResults[0].required {
		t.Fatalf("expected required FAIL on version drift, got %#v", driftedResults[0])
	}

	// Not installed yet: WARN (optional, not a hard prerequisite), never a
	// silent PASS.
	emptyRoot := t.TempDir()
	notInstalled := checkPgliteInstall(emptyRoot, toolchain)
	for _, r := range notInstalled {
		if r.status != "WARN" || r.required {
			t.Fatalf("expected optional WARN when not installed, got %#v", r)
		}
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
