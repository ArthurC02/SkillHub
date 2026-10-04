package localdrv

import (
	"context"
	"os"
	"strings"
	"testing"
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
