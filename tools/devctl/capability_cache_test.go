package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type countedRun struct {
	calls  int
	output string
	err    error
}

func (c *countedRun) run() ([]byte, error) {
	c.calls++
	return []byte(c.output), c.err
}

const someCapabilityTable = `[{"id":"search","needs":["SKILLHUB_SEARCH_URL"]}]`

func TestAnUnchangedPlatformReusesItsCapabilityTable(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedRun{output: someCapabilityTable}
	for range 2 {
		out, err := capabilityTable(cache, module, counted.run)
		if err != nil || string(out) != someCapabilityTable {
			t.Fatalf("table = %q, %v; want %q", out, err, someCapabilityTable)
		}
	}
	if counted.calls != 1 {
		t.Fatalf("ran %d times, want once", counted.calls)
	}
}

func TestAnEditedPlatformPrintsItsCapabilityTableAgain(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedRun{output: someCapabilityTable}
	if _, err := capabilityTable(cache, module, counted.run); err != nil {
		t.Fatal(err)
	}
	writeAt(t, module, "cmd/api/main.go", "package main\n")
	if _, err := capabilityTable(cache, module, counted.run); err != nil {
		t.Fatal(err)
	}
	if counted.calls != 2 {
		t.Fatalf("ran %d times, want twice", counted.calls)
	}
}

func TestAStoredCapabilityTableThatIsNotJSONIsPrintedAgain(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedRun{output: someCapabilityTable}
	if _, err := capabilityTable(cache, module, counted.run); err != nil {
		t.Fatal(err)
	}
	stored, err := filepath.Glob(filepath.Join(cache, "*"))
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored = %v, %v; want one entry", stored, err)
	}
	if err := os.WriteFile(stored[0], []byte(`[{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := capabilityTable(cache, module, counted.run)
	if err != nil || string(out) != someCapabilityTable || counted.calls != 2 {
		t.Fatalf("table = %q, %v after %d runs; want it printed again", out, err, counted.calls)
	}
}

func TestAFailedCapabilityRunIsNotKept(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	failing := &countedRun{err: errors.New("exit status 2")}
	if _, err := capabilityTable(cache, module, failing.run); err == nil {
		t.Fatal("a failed run returned no error")
	}
	counted := &countedRun{output: someCapabilityTable}
	if out, err := capabilityTable(cache, module, counted.run); err != nil || string(out) != someCapabilityTable || counted.calls != 1 {
		t.Fatalf("table = %q, %v after %d runs; want a fresh run", out, err, counted.calls)
	}
}
