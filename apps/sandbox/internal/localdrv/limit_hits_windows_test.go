package localdrv

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func runWithPrompt(t *testing.T, script, prompt string, lim sandbox.ResourceLimits) sandbox.Outcome {
	t.Helper()
	d, err := New(Config{NodeBin: requireNode(t), RunnerScript: testdataScript(t, script), BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	id := "hits"
	req := minimalRequest(id)
	req.TestCase.UserPrompt = prompt
	req.ResourceLimits = lim
	if err := d.Start(t.Context(), id, req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	outcome, err := d.Wait(ctx, id)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return outcome
}

func TestAJobProcessLimitIsReportedOnlyWhenTheWorkloadPassesIt(t *testing.T) {
	const maxPIDs = 32
	for _, tc := range []struct {
		name     string
		children int
		want     bool
	}{
		{"children fit under the limit", maxPIDs - 2, false},
		{"children exactly fill the limit", maxPIDs - 1, false},
		{"one child past the limit", maxPIDs, true},
		{"far past the limit", 2 * maxPIDs, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := runWithPrompt(t, "child-spawner.mjs", strconv.Itoa(tc.children), sandbox.ResourceLimits{
				MemoryBytes:       512 << 20,
				MaxPIDs:           maxPIDs,
				ArtifactFileBytes: 10 << 20,
			})
			if outcome.PidsLimitHit != tc.want {
				t.Errorf("PidsLimitHit = %v, want %v (output: %s)", outcome.PidsLimitHit, tc.want, outcome.Output)
			}
			if outcome.OOMKilled {
				t.Errorf("a process limit must not be reported as an OOM kill")
			}
		})
	}
}

func TestAJobMemoryLimitIsReportedWhenTheWorkloadOutgrowsIt(t *testing.T) {
	outcome := runWithPrompt(t, "memory-hog.mjs", "", sandbox.ResourceLimits{
		MemoryBytes:       192 << 20,
		ArtifactFileBytes: 10 << 20,
	})
	if !outcome.MemoryLimitHit {
		t.Errorf("MemoryLimitHit = false for a workload that outgrew its ceiling (exit %d, output: %s)", outcome.ExitCode, outcome.Output)
	}
	if outcome.ExitCode == 0 {
		t.Errorf("the hog is expected to die on its failed allocation, exit = 0 (output: %s)", outcome.Output)
	}
	if outcome.PidsLimitHit {
		t.Errorf("a memory limit must not be reported as a process limit")
	}
}

func TestAWorkloadWithinItsJobLimitsReportsNoLimitHit(t *testing.T) {
	outcome := runWithPrompt(t, "workload.mjs", "", sandbox.ResourceLimits{
		MemoryBytes:       512 << 20,
		MaxPIDs:           32,
		ArtifactFileBytes: 10 << 20,
	})
	if outcome.MemoryLimitHit || outcome.PidsLimitHit || outcome.OOMKilled {
		t.Errorf("a workload inside its limits was reported as hitting one: %+v", outcome)
	}
}
