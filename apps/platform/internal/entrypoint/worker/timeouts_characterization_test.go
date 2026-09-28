package worker

import (
	"runtime"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

func TestACreationStepJobIsAbandonedAfterThreeMinutes(t *testing.T) {
	if got := (&CreationStepWorker{}).Timeout(nil); got != 3*time.Minute {
		t.Errorf("creation step timeout = %v, want 3m", got)
	}
}

func TestARunExecutionJobIsAbandonedAfterFifteenMinutes(t *testing.T) {
	if got := (&RunExecuteWorker{}).Timeout(nil); got != 15*time.Minute {
		t.Errorf("run execution timeout = %v, want 15m", got)
	}
}

func TestTheDefaultQueueRunsAtMostFourJobsAtOnce(t *testing.T) {
	want := min(runtime.NumCPU(), 4)
	if got := riverConfig(river.NewWorkers(), nil, false).Queues[river.QueueDefault].MaxWorkers; got != want {
		t.Errorf("default queue workers = %d, want %d", got, want)
	}
}
