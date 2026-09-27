package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRefusesAnUnknownOptionBeforeTakingTheLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var out bytes.Buffer
	err := generate(root, []string{"--check", "--bogus"}, &out)
	if err == nil || err.Error() != `unknown gen option "--bogus"` {
		t.Fatalf("generate returned %v, want the unknown option named", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".devctl")); !os.IsNotExist(statErr) {
		t.Fatalf("an unknown option still touched .devctl: %v", statErr)
	}
}

func TestGenerateKeepsScratchAndReleasesTheLockWhenTheManifestHasNoImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := filepath.Join(root, "tools", "toolchain.yaml")
	writeAt(t, root, "tools/toolchain.yaml", "tools:\n  uv: 0.1.0\n")
	var out bytes.Buffer
	err := generate(root, []string{"--scope=sql"}, &out)
	if err == nil || err.Error() != manifest+" has no images entries" {
		t.Fatalf("generate returned %v, want the missing images section named", err)
	}
	scratch, found := strings.CutPrefix(strings.TrimSpace(out.String()), "generation failed; scratch kept at ")
	if !found {
		t.Fatalf("output %q does not say where the scratch was kept", out.String())
	}
	if info, statErr := os.Stat(scratch); statErr != nil || !info.IsDir() {
		t.Fatalf("the scratch it names is gone: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".devctl", "generate.lock")); !os.IsNotExist(statErr) {
		t.Fatalf("the generation lock outlived a failed run: %v", statErr)
	}
}
