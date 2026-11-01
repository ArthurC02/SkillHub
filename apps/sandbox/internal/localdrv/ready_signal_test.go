package localdrv

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestARunWhoseReadySignalCannotBeWrittenDoesNotStart(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(inputDir(work), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (&Driver{}).pushInputs(context.Background(), &run{workDir: work}, minimalRequest("ready-signal"))
	if err == nil || !strings.Contains(err.Error(), "inputs are ready") {
		t.Fatalf("pushInputs = %v, want the ready signal's failure: the workload would wait for inputs that never arrive", err)
	}
}

func TestTheStandInsAreWrittenBesideTheInputsOnlyWhenThereAreAny(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(inputDir(work), 0o700); err != nil {
		t.Fatal(err)
	}
	req := minimalRequest("stand-ins")
	req.MCPTools = []sandbox.MCPToolStandIn{{Reference: "mcp__calendar__create_event", Purpose: "Creates an event.", Parameters: "title"}}
	if err := (&Driver{}).pushInputs(context.Background(), &run{workDir: work}, req); err != nil {
		t.Fatalf("pushInputs: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(inputDir(work), "mcp-tools.json"))
	want := `[{"reference":"mcp__calendar__create_event","purpose":"Creates an event.","parameters":"title"}]`
	if err != nil || string(got) != want {
		t.Errorf("mcp-tools.json = %q (%v), want %s", got, err, want)
	}

	bare := t.TempDir()
	if err := os.MkdirAll(inputDir(bare), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := (&Driver{}).pushInputs(context.Background(), &run{workDir: bare}, minimalRequest("no-stand-ins")); err != nil {
		t.Fatalf("pushInputs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inputDir(bare), "mcp-tools.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a run with no stand-ins has mcp-tools.json (%v), want none", err)
	}
}

func TestARunWhoseStandInsCannotBeWrittenDoesNotStart(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(inputDir(work), "mcp-tools.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	req := minimalRequest("stand-ins-unwritable")
	req.MCPTools = []sandbox.MCPToolStandIn{{Reference: "mcp__calendar__create_event", Purpose: "Creates an event."}}
	err := (&Driver{}).pushInputs(context.Background(), &run{workDir: work}, req)
	if err == nil || !strings.Contains(err.Error(), "MCP tool stand-ins") {
		t.Fatalf("pushInputs = %v, want the stand-ins' failure: the agent would run without the tools its Skill declares", err)
	}
	if _, err := os.Stat(filepath.Join(inputDir(work), "ready")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the ready signal was written (%v) although the stand-ins were not", err)
	}
}
