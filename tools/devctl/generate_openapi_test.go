package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACodegenImageIsReusedOnlyWhenCIDeclaresItPrebuiltAndItIsPresent(t *testing.T) {
	t.Parallel()
	present := func(image string) bool { return image == "skillhub/ts-codegen:0.1.0" }
	cases := []struct {
		name     string
		prebuilt string
		image    string
		want     bool
	}{
		{"prebuilt and present", "true", "skillhub/ts-codegen:0.1.0", true},
		{"prebuilt but the image is missing", "true", "skillhub/ts-codegen:0.2.0", false},
		{"present but nobody declared it prebuilt", "", "skillhub/ts-codegen:0.1.0", false},
		{"a cache miss reports false, not empty", "false", "skillhub/ts-codegen:0.1.0", false},
	}
	for _, tc := range cases {
		if got := reuseCodegenImage(tc.prebuilt, tc.image, present); got != tc.want {
			t.Errorf("%s: reuseCodegenImage(%q, %q) = %v, want %v", tc.name, tc.prebuilt, tc.image, got, tc.want)
		}
	}
}

func TestGenerateTypeScriptOpenAPIFailsFastWhenScratchIsUnusable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch-is-a-file")
	if err := os.WriteFile(scratch, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := generateTypeScriptOpenAPI(root, scratch, "example-image", io.Discard); err == nil {
		t.Fatal("generateTypeScriptOpenAPI() = nil error, want a failure because scratch is not a directory")
	}
}

func TestContractLimitsIsSkippedWhenTypeScriptGenerationFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch-is-a-file")
	if err := os.WriteFile(scratch, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	images := map[string]string{"ts_codegen": "ts", "python_codegen": "py", "go_codegen": "go"}
	jobs, err := openAPIGenerationJobs(root, scratch, map[string]string{"datamodel_code_generator": "1"}, images)
	if err != nil || len(jobs) != 4 {
		t.Fatalf("openAPIGenerationJobs() = %d jobs, %v; want 4 jobs", len(jobs), err)
	}
	if _, err := jobs[0](io.Discard); err == nil {
		t.Fatal("the TypeScript job succeeded on an unusable scratch directory")
	}
	_, err = jobs[3](io.Discard)
	if err == nil || !strings.Contains(err.Error(), "skipped because TypeScript OpenAPI generation failed") {
		t.Fatalf("contract limits job = %v, want it skipped after the TypeScript failure", err)
	}
}

func TestValidateGeneratedContentRejectsTheRepoAbsolutePath(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	generated := filepath.Join(root, "generated")
	writeTestFile(t, generated, "models.go", "package models\n")
	if err := validateGeneratedContent(generated, repoRoot); err != nil {
		t.Fatalf("a clean file was rejected: %v", err)
	}
}
