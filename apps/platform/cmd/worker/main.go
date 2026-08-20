// Command worker consumes the Postgres job queue (ADR-010 deployment unit 3).
// Go is the only queue consumer (ADR-016 rule 3 / iron rule 7): Python is called
// over internal HTTP by a job, it never subscribes to a queue itself.
//
// This process's composition root is buildWorkers below, not apiserver.NewApp —
// that one wires the API's graph and this one wires the worker's, and the two
// deployment units share no object (ADR-032 §5 實作註記). main() keeps what is
// genuinely the process: reading the environment, starting the queue client and
// shutting it down. buildWorkers does no I/O, which is what lets main_test.go
// check the wiring without a database — the check that was missing when this
// file forgot to set run.Service.Queue and every run finished un-cleaned.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/eval"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/packaging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trace"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := queue.EnsureSchema(ctx, pool); err != nil {
		slog.Error("queue schema", "error", err)
		os.Exit(1)
	}

	// The sandbox fleet is deployment-static configuration (RUN-005); an empty one
	// is a valid deployment, in which every run fails saying so.
	providers := run.NewRegistryFromEnv()
	names := make([]string, 0, len(providers.Providers))
	for _, p := range providers.Providers {
		names = append(names, p.Name) // names only: the bearer tokens never reach a log
	}
	if len(names) == 0 {
		slog.Warn("no sandbox provider configured; runs will fail at dispatch")
	} else {
		slog.Info("sandbox providers configured", "providers", names)
	}
	// TRACE-002: the worker is what builds a RunRequest, so it is the process
	// that has to be able to mint an ingestion URL. Both halves must be set for
	// collection to happen: the secret this signs with, and the origin an
	// execution node can actually reach the control plane on.
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	traceBase := os.Getenv("SKILLHUB_TRACE_INGEST_URL")
	if !traceSigner.Enabled() || traceBase == "" {
		slog.Warn("trace ingestion not configured; sandboxes will be dispatched with no trace destination",
			"has_secret", traceSigner.Enabled(), "has_url", traceBase != "")
	}
	// SBX-008: the worker is what builds a RunRequest, so it is the process that
	// mints the short-lived object authorizations a sandbox is dispatched with.
	store, err := objstore.FromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
	}

	// ADR-017 / iron rule 8: the only model exit. Without it no Virtual Key is
	// minted, the egress allow list stays empty and sandboxes run with no route
	// out - which is a working deployment, just one that cannot call a model.
	gateway := run.GatewayFromEnv()
	if gateway == nil {
		slog.Warn("no model gateway configured; runs will be dispatched with no model credential")
	} else {
		// The address, never the key (iron rule 11).
		slog.Info("model gateway configured", "sandbox_base_url", gateway.SandboxBaseURL, "model", gateway.Model)
	}

	// EVAL-001's task-effect leg lives in apps/llm and is reached over internal
	// HTTP by this worker (iron rule 7). Without LLM_SERVICE_URL there is no judge:
	// runs still get an evaluation row, the deterministic findings are still
	// written, and the row is recorded as `failed` saying the judgement could not
	// be produced - which is a visible state, not a lenient verdict.
	var llm *llmclient.Client
	if llmURL := os.Getenv("LLM_SERVICE_URL"); llmURL != "" {
		token := os.Getenv("LLM_SERVICE_TOKEN")
		if token == "" {
			slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
			os.Exit(1)
		}
		llm = &llmclient.Client{BaseURL: llmURL, Token: token}
		slog.Info("judge service configured", "url", llmURL)
	} else {
		slog.Warn("LLM_SERVICE_URL not set; evaluations will be recorded as failed with no task verdict")
	}

	set, err := buildWorkers(pool, workerDeps{
		Providers:          providers,
		Store:              store,
		Gateway:            gateway,
		TraceSigner:        traceSigner,
		TraceIngestBaseURL: traceBase,
		LLM:                llm,
	})
	if err != nil {
		slog.Error("worker composition", "error", err)
		os.Exit(1)
	}

	if err := set.Queue.Start(ctx); err != nil {
		slog.Error("queue start", "error", err)
		os.Exit(1)
	}
	go metrics.Serve(os.Getenv("METRICS_ADDR"))
	slog.Info("worker started")

	<-ctx.Done()

	// Stop waits for jobs already running to finish; the context they were given
	// is already cancelled, so a job that respects cancellation exits promptly.
	if err := set.Queue.Stop(context.Background()); err != nil {
		slog.Error("queue stop", "error", err)
	}
	slog.Info("worker stopped")
}

// workerDeps is everything the environment supplies to this process's wiring.
// Deployment inputs only, in the same spirit as apiserver.Config: reading them is
// main's job, and building a domain Service out of them is buildWorkers'.
type workerDeps struct {
	Providers *run.Registry
	Store     *objstore.Client
	Gateway   *run.Gateway
	// TraceSigner and TraceIngestBaseURL are one setting in two halves: either
	// both are configured or no trace is collected.
	TraceSigner        *trace.Signer
	TraceIngestBaseURL string
	// LLM is the internal Python service. Nil is a working deployment with no
	// judge and no suggester.
	LLM *llmclient.Client
}

// workerSet is the wired graph this process runs. Exposed field by field rather
// than returned as a started client, because "which dependency reached which
// service" is exactly what the wiring test has to be able to look at.
type workerSet struct {
	Runs        *run.Service
	Evaluations *eval.Service
	RunEvents   *eval.RunEventConsumer
	Events      *outbox.Dispatcher
	Objects     *objreconcile.Service
	Queue       *river.Client[pgx.Tx]
	// WorkerKinds is every job kind that has a worker registered, and Scheduled is
	// every kind this process enqueues on a timer. Recorded while wiring because
	// River's registry cannot be read back: a periodic job whose worker was
	// dropped fails only when the insert is attempted, one interval into a
	// deployment, and only in the log.
	WorkerKinds map[string]bool
	Scheduled   []string
}

// buildWorkers wires this process's object graph: no environment reads, no
// database round trips, no goroutines. Everything here is a struct literal, a
// registration or a back-assignment, so the failure it exists to prevent — a
// dependency nobody noticed was never set — is reachable from a test.
func buildWorkers(pool *pgxpool.Pool, deps workerDeps) (*workerSet, error) {
	set := &workerSet{WorkerKinds: map[string]bool{}}

	set.Runs = &run.Service{
		Pool: pool, Providers: deps.Providers, Store: deps.Store, Gateway: deps.Gateway,
		TraceSigner: deps.TraceSigner, TraceIngestBaseURL: deps.TraceIngestBaseURL,
	}

	// The Trace context, injected rather than built inside eval's own methods
	// (ADR-032 §5). Same signer as the dispatcher above, so the process has one
	// Trace configuration and not two.
	set.Evaluations = &eval.Service{
		Pool: pool, Store: deps.Store,
		Trace: &trace.Service{Pool: pool, Signer: deps.TraceSigner},
	}
	if deps.LLM != nil {
		set.Evaluations.Judge = deps.LLM
		// EVAL-002's proposal leg, same service and same gateway. Without it a run
		// still gets a verdict; it simply gets no advice, which is a complete
		// evaluation and not a failed one.
		set.Evaluations.Suggester = deps.LLM
	}

	// DDD-005: the only trigger for an evaluation. A terminal run transition
	// announces `run.succeeded` / `run.failed` in its own transaction and the
	// outbox hands that event to this consumer, so internal/run does not have to
	// know evaluation exists. Insert is filled in below, once the client the whole
	// worker set is registered with exists.
	set.RunEvents = &eval.RunEventConsumer{Current: gen.New(pool).GetCurrentEvaluation}

	// Who listens to what, as data rather than as one callback (DDD review P2).
	// Every event type in the catalogue is either claimed by a consumer here or
	// declared uninteresting to this process with a reason, and Validate refuses
	// the wiring if one is neither — a dropped consumer stops the process at boot
	// instead of turning into a week of runs that were published to nobody.
	set.Events = outbox.NewDispatcher().
		On("evaluation", set.RunEvents.Deliver, outbox.RunSucceeded, outbox.RunFailed).
		Ignore("progress announcements: a run that is still moving is read from its own row by the UI, and no worker-side reaction is owed",
			outbox.RunQueued, outbox.RunProvisioning, outbox.RunPreparing,
			outbox.RunRunning, outbox.RunEvaluating).
		Ignore("terminal with nothing to judge: the run was stopped before it could produce what the criteria are about, so evaluation skips it by design",
			outbox.RunCancelled, outbox.RunTimedOut).
		Ignore("cleanup outcome is already recorded on the run row and alerted on through metrics; nothing in this process reacts to it",
			outbox.RunCleanupCleaned, outbox.RunCleanupFailed)
	if err := set.Events.Validate(); err != nil {
		return nil, fmt.Errorf("outbox dispatch wiring: %w", err)
	}
	outboxWorker := &outbox.Worker{Pool: pool, Deliver: set.Events.Deliver}

	workers := river.NewWorkers()
	// Every job kind the platform knows about is registered here. A kind with no
	// worker is not a silent no-op — River fails the job — which is the behaviour
	// we want if a deploy ever drops one.
	addWorker(set, workers, &run.Worker{Svc: set.Runs})
	addWorker(set, workers, &run.CleanupWorker{Svc: set.Runs})
	addWorker(set, workers, &run.OrphanScanWorker{Svc: set.Runs})
	addWorker(set, workers, &run.SuperviseWorker{Svc: set.Runs})
	addWorker(set, workers, &eval.Worker{Svc: set.Evaluations})
	addWorker(set, workers, &eval.RecoveryWorker{Svc: set.Evaluations})
	addWorker(set, workers, outboxWorker)
	// SEC-006 retention and 04 丙-9's object-existence check, one sweep: expired
	// download packages lose their bytes, and rows whose object has gone missing
	// stop claiming it is there.
	//
	// The sweep finds the discrepancies; the two row corrections are the owners'
	// own writes and are injected here (ADR-033 clearance path 4). objreconcile is
	// a generic scanner and must not import packaging or testlab, so this
	// composition root is the only place the three meet. Sweep fails closed if
	// either is left unset — see main_test.go.
	set.Objects = &objreconcile.Service{
		Pool: pool, Store: deps.Store,
		RecordArtifactPurged: packaging.MarkArtifactPurged,
		RecordDatasetLost:    testlab.MarkDatasetObjectLost,
	}
	addWorker(set, workers, &objreconcile.Worker{Svc: set.Objects})

	// Periodic jobs run on the elected leader only, so several worker processes
	// do not each sweep. RunOnStart is what makes the supervisor the restart
	// recovery path (RUN-008) and not merely a watchdog.
	var periodic []*river.PeriodicJob
	schedule := func(args river.JobArgs, every time.Duration, runOnStart bool) {
		set.Scheduled = append(set.Scheduled, args.Kind())
		var opts *river.PeriodicJobOpts
		if runOnStart {
			opts = &river.PeriodicJobOpts{RunOnStart: true}
		}
		periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(every),
			func() (river.JobArgs, *river.InsertOpts) { return args, nil }, opts))
	}
	schedule(eval.RecoveryArgs{}, eval.RecoveryInterval, true)
	schedule(run.SuperviseArgs{}, run.SuperviseInterval, true)
	schedule(run.OrphanScanArgs{}, run.OrphanScanInterval, true)
	// RunOnStart because an evaluation now waits on this drain: a restart that
	// left events in the backlog should not cost the user a full interval before
	// their finished run gets a verdict.
	schedule(outbox.PublishArgs{}, outboxWorker.Interval(), true)
	// No RunOnStart, unlike the four above: this one is not recovering from a
	// restart, and a deploy loop would otherwise re-probe every stored object on
	// every rollout. An hour late is on time here.
	schedule(objreconcile.Args{}, objreconcile.Interval, false)

	client, err := queue.New(pool, &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			// One queue until there is a measured reason for more. Concurrency is
			// bounded well below the per-workspace limit of 2 concurrent runs
			// (PDM-005 §5.2); real capacity planning lands with the first load test.
			river.QueueDefault: {MaxWorkers: min(runtime.NumCPU(), 4)},
		},
		PeriodicJobs: periodic,
	})
	if err != nil {
		return nil, fmt.Errorf("queue client: %w", err)
	}
	set.Queue = client

	// The worker enqueues jobs as well as working them: a terminal transition owes
	// a cleanup, and the supervisor's backlog sweep re-enqueues the ones that were
	// missed (RUN-007). Both are guarded by `s.Queue != nil`, so leaving this unset
	// meant every run finished un-cleaned — its sandbox and its Virtual Key
	// surviving the run — with nothing in the log to say so.
	set.Runs.Queue = client
	set.RunEvents.Insert = client.Insert
	return set, nil
}

// addWorker registers one worker and remembers the kind it claims, which is the
// only way to get that list back out: River's registry is unexported.
func addWorker[T river.JobArgs](set *workerSet, workers *river.Workers, worker river.Worker[T]) {
	var args T
	set.WorkerKinds[args.Kind()] = true
	river.AddWorker(workers, worker)
}
