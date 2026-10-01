package worker

import (
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
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

func TestAnEvaluationJobOutlastsItsJudgeAndSuggestionCalls(t *testing.T) {
	calls := eval.JudgeBudget.Deadline + eval.SuggestImprovementsBudget.Deadline
	if got := (&EvaluationExecuteWorker{}).Timeout(nil); got <= calls {
		t.Errorf("evaluation job timeout = %v, want more than the %v its two model calls may take", got, calls)
	}
}

func TestAnEnrichmentBackfillJobOutlastsRiversOneMinuteDefault(t *testing.T) {
	if got := (&EnrichmentBackfillWorker{}).Timeout(nil); got != 15*time.Minute {
		t.Errorf("enrichment backfill timeout = %v, want 15m", got)
	}
}

func TestTheWorkerServesEveryDeclaredQueueAndEveryPeriodicJobsQueue(t *testing.T) {
	queues := riverConfig(river.NewWorkers(), nil, false).Queues
	for name := range wiring.Queues {
		if queues[name].MaxWorkers < 1 {
			t.Errorf("queue %q is declared but the worker runs no job from it", name)
		}
	}
	for kind, queue := range periodicQueue {
		if queues[queue].MaxWorkers < 1 {
			t.Errorf("periodic job %s goes to queue %q, which the worker does not serve", kind, queue)
		}
	}
	if periodicQueue[RunSuperviseArgs{}.Kind()] != wiring.QueueRuns {
		t.Error("run supervision does not run on the run queue, so model jobs can delay it")
	}
}

func TestPeriodicRunAndEnrichmentJobsAreInsertedOnTheirOwnQueues(t *testing.T) {
	for args, want := range map[river.JobArgs]string{
		RunSuperviseArgs{}:       wiring.QueueRuns,
		RunOrphanScanArgs{}:      wiring.QueueRuns,
		EnrichmentBackfillArgs{}: wiring.QueueModel,
		PartitionCreateArgs{}:    "",
	} {
		if got := periodicInsert(args).Queue; got != want {
			t.Errorf("periodic %s is inserted on queue %q, want %q", args.Kind(), got, want)
		}
	}
}
