package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACodexAgentCarriesTheNameAndDescriptionItWasGivenInThatOrder(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "agents")
	agent := "---\nname: the-writer\ndescription: writes files\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(source, "writer.md"), []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexAgents(source, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "writer.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "name = \"the-writer\"\ndescription = \"writes files\"\ndeveloper_instructions = \"Body.\\n\"\n"
	if !strings.HasSuffix(string(data), want) {
		t.Fatalf("got:\n%s\nwant it to end with:\n%s", data, want)
	}
}
