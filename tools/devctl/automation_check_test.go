package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskDescriptionsFindsMissingDescriptions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "Taskfile.yml")
	contents := `version: "3"
tasks:
  documented:
    desc: Safe task
    cmd: echo ok
  missing:
    cmd: echo hidden
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskDescriptions(path)
	if err != nil {
		t.Fatal(err)
	}
	if tasks["documented"] != "Safe task" {
		t.Fatalf("documented description = %q", tasks["documented"])
	}
	if description, ok := tasks["missing"]; !ok || description != "" {
		t.Fatalf("missing task was not detected: %#v", tasks)
	}
}
