package wiring

import (
	"testing"

	"github.com/riverqueue/river"
)

func TestEveryQueueAJobIsSentToHasWorkers(t *testing.T) {
	for name, opts := range map[string]*river.InsertOpts{
		"run execute": executeOptions(), "run cleanup": cleanupOptions(),
		"creation step": creationStepOptions(), "evaluation": evaluationOptions(),
	} {
		if queue, ok := Queues[opts.Queue]; !ok || queue.MaxWorkers < 1 {
			t.Errorf("%s jobs go to queue %q, which no worker serves", name, opts.Queue)
		}
	}
}

func TestModelJobsCannotTakeTheWorkersThatAdvanceRuns(t *testing.T) {
	runs := map[string]bool{executeOptions().Queue: true, cleanupOptions().Queue: true}
	for name, opts := range map[string]*river.InsertOpts{"creation step": creationStepOptions(), "evaluation": evaluationOptions()} {
		if runs[opts.Queue] || opts.Queue == river.QueueDefault {
			t.Errorf("%s jobs share queue %q with run or housekeeping jobs; a burst of model calls would stall them", name, opts.Queue)
		}
	}
	if runs[river.QueueDefault] {
		t.Error("run jobs share the default queue with housekeeping jobs")
	}
}
