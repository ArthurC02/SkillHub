package worker

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type Deps struct {
	CreationLimits creation.Limits
	Providers      *run.Registry
	Store          *objstore.Client
	Gateway        *run.Gateway

	TraceSigner        *trace.Signer
	TraceIngestBaseURL string

	LLM *llmclient.Client

	PollOnly bool
}

type Set struct {
	Creation    *creation.Service
	Runs        *run.Service
	Evaluations *eval.Service
	Packaging   *packaging.Service
	RunEvents   *eval.RunEventConsumer
	Events      *outbox.Dispatcher
	Objects     *objreconcile.Service
	Queue       *river.Client[pgx.Tx]

	WorkerKinds map[string]bool
	Scheduled   map[string]bool
}

func packagingCandidates(list func(context.Context, int32) ([]packaging.ReconcileCandidate, error)) objreconcile.ListFunc {
	return func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
		rows, err := list(ctx, limit)
		if err != nil {
			return nil, err
		}
		out := make([]objreconcile.Candidate, len(rows))
		for i, row := range rows {
			out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
		}
		return out, nil
	}
}

func datasetCandidates(list func(context.Context, int32) ([]testlab.ReconcileCandidate, error)) objreconcile.ListFunc {
	return func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
		rows, err := list(ctx, limit)
		if err != nil {
			return nil, err
		}
		out := make([]objreconcile.Candidate, len(rows))
		for i, row := range rows {
			out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
		}
		return out, nil
	}
}

func BuildWorkers(pool *pgxpool.Pool, deps Deps) (*Set, error) {
	set := &Set{WorkerKinds: map[string]bool{}, Scheduled: map[string]bool{}}
	downloads := &packaging.Service{Pool: pool}
	set.Packaging = downloads
	registrySvc := &registry.Service{Pool: pool}
	testlabSvc := &testlab.Service{Pool: pool}
	downloads.TestLab = testlabSvc

	set.Runs = &run.Service{
		Pool: pool, Providers: deps.Providers, Store: deps.Store, Gateway: deps.Gateway,
		TestLab:     testlabSvc,
		TraceSigner: deps.TraceSigner, TraceIngestBaseURL: deps.TraceIngestBaseURL,
		ActiveArtifactReferences: downloads.ActiveArtifactReferences,
	}
	wiring.WireRunRegistryReaders(set.Runs, registrySvc)
	traceSvc := wiring.NewTraceService(pool, deps.TraceSigner, set.Runs)
	set.Runs.Trace = traceSvc

	set.Evaluations = &eval.Service{
		Pool: pool, Store: deps.Store,
		Trace: traceSvc, TestLab: testlabSvc,
	}
	wiring.WireEvaluationRunReaders(set.Evaluations, set.Runs)
	wiring.WireEvaluationRegistryReaders(set.Evaluations, registrySvc)
	if deps.LLM != nil {
		set.Evaluations.Judge = deps.LLM

		set.Evaluations.Suggester = deps.LLM
	}

	set.RunEvents = &eval.RunEventConsumer{HasCurrentEvaluation: set.Evaluations.HasCurrentEvaluation}

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

	set.Creation = &creation.Service{Pool: pool, Limits: deps.CreationLimits, LLM: deps.LLM}
	creationVersions := &ingest.Service{Pool: pool, Store: deps.Store, References: registrySvc}
	creationSearch := &catalog.Service{Pool: pool, LLM: deps.LLM}
	wireCreationReads(set.Creation, creationVersions, creationSearch)
	wireCreationGateway(set.Creation, deps.Gateway)
	wireCreationFetch(set.Creation)

	creditSvc, err := wiring.NewCreditService(pool)
	if err != nil {
		return nil, fmt.Errorf("credit wiring: %w", err)
	}
	creditSvc.Config.SessionIdle = deps.CreationLimits.SessionTimeout
	wiring.WireCreationCredit(set.Creation, creditSvc, pool)
	backfillSvc := newBackfillService(pool, deps)
	wireCostRecording(creditSvc, creationSearch, creationVersions, backfillSvc, set.Evaluations)
	wiring.WireCreditDisplay(creditSvc, set.Runs, traceSvc, set.Evaluations)
	wiring.WireRunCredit(set.Runs, creditSvc, pool)
	workers := river.NewWorkers()
	addWorker(set, workers, &creation.Worker{Svc: set.Creation})
	addWorker(set, workers, &creation.ExpiryWorker{Svc: set.Creation})

	addWorker(set, workers, &run.Worker{Svc: set.Runs})
	addWorker(set, workers, &run.CleanupWorker{Svc: set.Runs})
	addWorker(set, workers, &run.OrphanScanWorker{Svc: set.Runs})
	addWorker(set, workers, &run.SuperviseWorker{Svc: set.Runs})
	addWorker(set, workers, &eval.Worker{Svc: set.Evaluations})
	addWorker(set, workers, &eval.RecoveryWorker{Svc: set.Evaluations})
	addWorker(set, workers, outboxWorker)

	set.Objects = &objreconcile.Service{
		Pool: pool, Store: deps.Store,
		ListExpiredArtifacts:       packagingCandidates(downloads.ExpiredReconcileCandidates),
		ListDownloadIntents:        packagingCandidates(downloads.DownloadCleanupIntentCandidates),
		ListClaimedArtifacts:       packagingCandidates(downloads.ClaimedReconcileCandidates),
		ListClaimedDatasets:        datasetCandidates(testlabSvc.ClaimedReconcileCandidates),
		RecordArtifactPurged:       downloads.MarkArtifactPurged,
		RecordDownloadIntentPurged: downloads.MarkDownloadCleanupIntentPurged,
		RecordDatasetLost:          testlabSvc.MarkDatasetObjectLost,
		GuardArtifactRemoval:       downloads.GuardArtifactRemoval,
	}
	addWorker(set, workers, &objreconcile.Worker{Svc: set.Objects})

	addWorker(set, workers, &PartitionCreateWorker{Pool: pool})
	addWorker(set, workers, &EnrichmentBackfillWorker{Svc: backfillSvc})

	addWorker(set, workers, &credit.RecomputeWorker{Svc: creditSvc})

	var periodic []*river.PeriodicJob
	schedule := func(args river.JobArgs, every time.Duration, runOnStart bool) {
		set.Scheduled[args.Kind()] = runOnStart
		var opts *river.PeriodicJobOpts
		if runOnStart {
			opts = &river.PeriodicJobOpts{RunOnStart: true}
		}
		periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(every),
			func() (river.JobArgs, *river.InsertOpts) { return args, nil }, opts))
	}
	schedule(eval.RecoveryArgs{}, eval.RecoveryInterval, true)
	schedule(run.SuperviseArgs{}, run.SuperviseInterval, true)
	if deps.CreationLimits.Valid() {
		schedule(creation.ExpiryArgs{}, time.Minute, true)
	}
	schedule(run.OrphanScanArgs{}, run.OrphanScanInterval, true)

	schedule(outbox.PublishArgs{}, outboxWorker.Interval(), true)

	schedule(objreconcile.Args{}, objreconcile.Interval, false)

	for _, kind := range creditStatKinds {
		schedule(credit.RecomputeArgs{StatKind: kind, WindowSeconds: int64(creditStatWindow / time.Second)}, 24*time.Hour, false)
	}

	schedule(PartitionCreateArgs{}, PartitionCreateInterval, true)

	schedule(EnrichmentBackfillArgs{}, EnrichmentBackfillInterval, false)

	client, err := queue.New(pool, riverConfig(workers, periodic, deps.PollOnly))
	if err != nil {
		return nil, fmt.Errorf("queue client: %w", err)
	}
	set.Queue = client
	set.Creation.Insert = func(ctx context.Context, tx pgx.Tx, a creation.JobArgs) error {
		_, err := client.InsertTx(ctx, tx, a, &river.InsertOpts{MaxAttempts: 1})
		return err
	}

	set.Runs.Queue = client
	set.RunEvents.Insert = client.Insert
	return set, nil
}

func riverConfig(workers *river.Workers, periodic []*river.PeriodicJob, pollOnly bool) *river.Config {
	return &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{

			river.QueueDefault: {MaxWorkers: min(runtime.NumCPU(), 4)},
		},
		PeriodicJobs: periodic,
		PollOnly:     pollOnly,
	}
}

func addWorker[T river.JobArgs](set *Set, workers *river.Workers, worker river.Worker[T]) {
	var args T
	set.WorkerKinds[args.Kind()] = true
	river.AddWorker(workers, worker)
}
