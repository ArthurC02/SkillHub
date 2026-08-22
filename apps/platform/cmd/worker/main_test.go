package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// The wiring test this process did not have when it shipped without
// run.Service.Queue: every run finished, none was cleaned up, and nothing said
// so. A missing dependency here is silent by construction — the fields are all
// optional at the type level and most of them are legitimately nil in some
// deployment — so the only thing that can catch it is an assertion that this
// process, specifically, sets them.
//
// No database and no object storage are touched: pgxpool.New does not dial (it
// opens connections lazily) and buildWorkers is a pile of struct literals.

func testDeps(t *testing.T) (*pgxpool.Pool, workerDeps) {
	t.Helper()
	// A parseable DSN nothing listens on. River refuses a client with queues and
	// no pool, so the pool has to exist; it is never queried.
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	store, err := objstore.New("localhost:8333", "key", "secret", "skillhub", false)
	if err != nil {
		t.Fatalf("objstore.New: %v", err)
	}
	return pool, workerDeps{
		Providers:          &run.Registry{},
		Store:              store,
		TraceSigner:        &trace.Signer{Secret: []byte("test")},
		TraceIngestBaseURL: "https://control.invalid",
		LLM:                &llmclient.Client{BaseURL: "https://llm.invalid", Token: "t"},
	}
}

func TestBuildWorkersInjectsEveryDependencyThisProcessOwns(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := buildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("buildWorkers: %v", err)
	}

	// The run service dispatches, so it needs the fleet, the bytes, the trace
	// credential — and the queue client, which is assigned after the client is
	// built and is therefore the one that has already been forgotten once.
	switch {
	case set.Runs.Pool == nil:
		t.Error("run service has no pool")
	case set.Runs.Queue == nil:
		t.Error("run service has no queue: cleanup and the supervisor's re-enqueue are silently skipped (RUN-007)")
	case set.Runs.Providers == nil:
		t.Error("run service has no provider registry")
	case set.Runs.Store == nil:
		t.Error("run service has no object store")
	case set.Runs.ActiveArtifactReferences == nil:
		t.Error("run service has no packaging artifact reference counter")
	case set.Runs.ReadSkill == nil || set.Runs.ReadVersion == nil:
		t.Error("run service has no Registry owner reads")
	case set.Runs.TestLab == nil:
		t.Error("run service has no Test Lab owner reads")
	case set.Runs.Trace == nil:
		t.Error("run service has no Trace masking activity reader")
	case set.Runs.TraceSigner == nil || set.Runs.TraceIngestBaseURL == "":
		t.Error("run service was wired with half a trace configuration")
	}

	switch {
	case set.Evaluations.Pool == nil || set.Evaluations.Store == nil:
		t.Error("evaluation service is missing its persistence")
	case set.Evaluations.Trace == nil:
		t.Error("evaluation service has no trace context")
	case set.Runs.Trace != set.Evaluations.Trace:
		t.Error("run and evaluation were not wired to the shared Trace service")
	case set.Evaluations.Trace.ReadRunState == nil || set.Evaluations.Trace.ReadIngestRunState == nil ||
		set.Evaluations.Trace.ReadRunTransitions == nil:
		t.Error("trace service is missing a Run-owned fact reader")
	case set.Evaluations.ReadRunFacts == nil || set.Evaluations.ReadEvaluationInput == nil:
		t.Error("evaluation service is missing Run-owned fact readers")
	case set.Evaluations.ReadVersion == nil || set.Evaluations.ReadLatestVersion == nil ||
		set.Evaluations.ReadSkill == nil || set.Evaluations.ReadRuntimeCompatibility == nil:
		t.Error("evaluation service is missing Registry-owned fact readers")
	case set.Evaluations.TestLab == nil:
		t.Error("evaluation service is missing Test Lab owner reads")
	case set.Evaluations.Judge == nil || set.Evaluations.Suggester == nil:
		t.Error("LLM was configured but the judge or the suggester did not get it")
	}
	if set.Packaging == nil || set.Packaging.TestLab == nil {
		t.Error("packaging service is missing Test Lab owner reads")
	} else if set.Runs.TestLab != set.Evaluations.TestLab || set.Runs.TestLab != set.Packaging.TestLab {
		t.Error("run, evaluation and packaging were not wired to the shared Test Lab service")
	}

	// The object reconciler's reads and corrections belong to packaging and testlab
	// (ADR-033 clearance path 4) and only this file puts them there. Unset, the
	// hourly sweep refuses to run, so expired download packages keep their bytes
	// and rows keep claiming objects that are gone.
	if set.Objects.ListExpiredArtifacts == nil || set.Objects.ListClaimedArtifacts == nil ||
		set.Objects.ListClaimedDatasets == nil || set.Objects.RecordArtifactPurged == nil ||
		set.Objects.RecordDatasetLost == nil {
		t.Error("object reconciler is missing an owner read/write function")
	}

	// The outbox consumer's two function fields: one reads the standing
	// evaluation, the other enqueues. A nil Insert panics on the first finished
	// run rather than at boot.
	if set.RunEvents.HasCurrentEvaluation == nil || set.RunEvents.Insert == nil {
		t.Error("run event consumer is missing HasCurrentEvaluation or Insert")
	}
}

// A deployment with no model service is a working one: no judge, no suggester,
// and nothing else changed. The nil-interface trap is what this guards — a nil
// *llmclient.Client assigned into an interface field is not nil, and every
// evaluation would panic instead of being recorded as unjudged.
func TestBuildWorkersLeavesTheJudgeUnsetWithoutAnLLM(t *testing.T) {
	pool, deps := testDeps(t)
	deps.LLM = nil
	set, err := buildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("buildWorkers: %v", err)
	}
	if set.Evaluations.Judge != nil || set.Evaluations.Suggester != nil {
		t.Error("no LLM service configured, yet the evaluation service holds a judge or a suggester")
	}
}

// Validate is what makes a dropped consumer a boot failure instead of events
// marked published that nobody acted on. buildWorkers already calls it; asserting
// it here names the failure, so an event type added to the catalogue without a
// decision about this process fails in a test rather than in a deploy.
func TestOutboxDispatchAccountsForEveryEventType(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := buildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("buildWorkers: %v", err)
	}
	if err := set.Events.Validate(); err != nil {
		t.Errorf("outbox dispatch leaves part of the catalogue unaccounted for: %v", err)
	}
}

// A periodic job whose worker was dropped is not a build error and not a startup
// error: River discovers it when the timer first fires, and reports it as one log
// line per interval forever.
func TestEveryScheduledJobHasAWorker(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := buildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("buildWorkers: %v", err)
	}
	if len(set.Scheduled) == 0 {
		t.Fatal("no periodic job is scheduled; the supervisor and the outbox drain are not running")
	}
	for _, kind := range set.Scheduled {
		if !set.WorkerKinds[kind] {
			t.Errorf("periodic job %q is scheduled but no worker is registered for it", kind)
		}
	}
}
