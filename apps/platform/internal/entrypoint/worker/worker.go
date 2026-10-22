package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/batch"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/evalsettings"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/llmadapter"
)

type Deps struct {
	CreationLimits creation.Limits
	Providers      *run.Registry
	Store          *objstore.Client
	Gateway        *run.Gateway
	RunDeployment  run.Deployment

	TraceSigner        *trace.Signer
	TraceIngestBaseURL string

	LLM *llmclient.Client

	RestoreRate capacity.RestoreRate

	PollOnly bool
}

type Set struct {
	Creation       *creation.Service
	Runs           *run.Service
	Evaluations    *eval.Service
	Batches        *batch.BatchService
	Packaging      *packaging.Service
	Registry       *registry.Service
	CreationSearch *catalog.Service
	RunEvents      *eval.RunEventConsumer
	BatchEvents    *batch.BatchEventConsumer
	SkillVersions  *eval.SkillVersionConsumer
	Events         *outbox.Dispatcher
	Objects        *objreconcile.Service
	Queue          *river.Client[pgx.Tx]

	WorkerKinds map[string]bool
	Scheduled   map[string]bool
	Gauges      []func(context.Context) error
}

type evaluationReaders struct {
	registry *registry.Service
	testlab  *testlab.Service
	trace    *trace.Service
	budgets  *modelbudget.Service
}

func addEvaluationServices(set *Set, pool *pgxpool.Pool, deps Deps, r evaluationReaders) {
	set.Evaluations = &eval.Service{
		Pool: pool, Store: deps.Store,
		Trace: r.trace, TestLab: r.testlab, Budgets: r.budgets,
	}
	wiring.WireEvaluation(set.Evaluations, set.Runs, r.registry)
	wireEvaluationModelAndEvents(set.Evaluations, pool, deps.LLM)
	set.Batches = wiring.NewEvaluationBatches(pool, set.Runs, r.testlab, r.trace)

	set.RunEvents = wiring.NewRunEventConsumer(pool, set.Evaluations, set.Runs)
	set.SkillVersions = &eval.SkillVersionConsumer{}
	set.BatchEvents = &batch.BatchEventConsumer{HasUnsettledBatch: set.Batches.HasUnsettledBatch}
}

func BuildWorkers(pool *pgxpool.Pool, deps Deps) (*Set, error) {
	set := &Set{WorkerKinds: map[string]bool{}, Scheduled: map[string]bool{}}
	downloads := &packaging.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
	set.Packaging = downloads
	registrySvc := &registry.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	set.Registry = registrySvc
	testlabSvc := &testlab.Service{Pool: pool, ClearSightings: objreconcile.ClearDatasetSightings}
	downloads.TestLab = testlabSvc

	set.Runs = newRunService(pool, deps, testlabSvc)
	wiring.WireRunRegistryReaders(set.Runs, registrySvc)
	traceSvc := wiring.NewTraceService(pool, deps.TraceSigner, set.Runs)
	set.Runs.Trace = traceSvc

	budgets := wiring.NewModelBudgets(pool)
	addEvaluationServices(set, pool, deps, evaluationReaders{registry: registrySvc, testlab: testlabSvc, trace: traceSvc, budgets: budgets})

	set.Events = newEventDispatcher(set.RunEvents, set.SkillVersions, set.BatchEvents, &publishing.SkillDeletedConsumer{Pool: pool})
	if err := set.Events.Validate(); err != nil {
		return nil, fmt.Errorf("outbox dispatch wiring: %w", err)
	}
	outboxWorker := &outbox.Worker{Pool: pool, Deliver: set.Events.Deliver}

	var creationVersions *ingest.Service
	set.Creation, creationVersions, set.CreationSearch = newCreationServices(pool, deps, registrySvc)
	set.CreationSearch.Budgets = budgets

	creditSvc, err := wiring.NewCreditService(pool)
	if err != nil {
		return nil, fmt.Errorf("credit wiring: %w", err)
	}
	creditSvc.Config.SessionIdle = deps.CreationLimits.SessionTimeout
	wiring.WireCreationCredit(set.Creation, creditSvc, pool)
	halted := set.Runs.DispatchGate().ModelCallsHalted
	set.Creation.Billing.ModelCallsHalted = halted
	creationVersions.Billing.ModelCallsHalted = halted
	backfillSvc := newBackfillService(pool, deps, halted)
	wireCostRecording(creditSvc, set.CreationSearch, creationVersions, backfillSvc, set.Evaluations)
	wiring.WireCreditDisplay(creditSvc, set.Runs, traceSvc, set.Evaluations)
	wiring.WireRunCredit(set.Runs, creditSvc, pool)
	workers := river.NewWorkers()
	addDomainWorkers(set, workers)
	addWorker(set, workers, outboxWorker)

	set.Objects = newObjectReconciler(pool, deps.Store, downloads, testlabSvc)
	addWorker(set, workers, &objreconcile.Worker{Svc: set.Objects})

	addWorker(set, workers, &PartitionCreateWorker{Pool: pool})
	addWorker(set, workers, &EnrichmentBackfillWorker{Svc: backfillSvc})
	addGaugePublishers(set, workers, outboxWorker, map[string]backlogOldest{
		metrics.BacklogOrphanObjects: registrySvc.OldestCollectableObject,
		metrics.BacklogSourceChecks:  creationVersions.OldestSourceCheck,
		metrics.BacklogEnrichment:    (&catalog.EnrichmentBackfill{Pool: pool}).OldestPending,
	})
	addCapacityObserver(set, workers, capacity.Store{Pool: pool, Rate: deps.RestoreRate})

	addCreditConsumers(set, workers, pool, deps, creditSvc)

	client, err := queue.New(pool, riverConfig(workers, periodicJobs(set, deps, outboxWorker), deps.PollOnly))
	if err != nil {
		return nil, fmt.Errorf("queue client: %w", err)
	}
	connectQueue(set, client)
	return set, nil
}

func newRunService(pool *pgxpool.Pool, deps Deps, testlabSvc *testlab.Service) *run.Service {
	return &run.Service{
		Pool: pool, Providers: deps.Providers, Store: deps.Store, Gateway: run.GatewayOrNone(deps.Gateway),
		ClearSightings: objreconcile.ClearArtifactSightings,
		TestLab:        testlabSvc,
		TraceSigner:    deps.TraceSigner, TraceIngestBaseURL: deps.TraceIngestBaseURL,
		ActiveArtifactReferences: packaging.ActiveArtifactReferences,
		LastOrphanScan:           wiring.LastOrphanScan(pool),
		Deployment:               deps.RunDeployment,
	}
}

func wireEvaluationModelAndEvents(evaluations *eval.Service, pool *pgxpool.Pool, llm *llmclient.Client) {
	evaluations.ReadEventsOfType = func(ctx context.Context, page outbox.EventPage) ([]outbox.Event, error) {
		return outbox.EventsOfType(ctx, pool, page)
	}
	evaluations.Judge = llmadapter.JudgeOrNone(llm)
	evaluations.PanelEnabled = (&evalsettings.Service{Pool: pool}).JudgePanelEnabled
	evaluations.Suggester = llmadapter.SuggesterOrNone(llm)
}

func newEventDispatcher(
	runEvents *eval.RunEventConsumer, skillVersions *eval.SkillVersionConsumer, batchEvents *batch.BatchEventConsumer,
	deletedSkills *publishing.SkillDeletedConsumer,
) *outbox.Dispatcher {
	return outbox.NewDispatcher().
		On("evaluation", runEvents.Deliver, outbox.RunSucceeded, outbox.RunFailed).
		On("suggestions applied", skillVersions.Deliver, outbox.SkillVersionAdded).
		On("evaluation batches", batchEvents.Deliver, outbox.RunSucceeded, outbox.RunFailed, outbox.RunCancelled, outbox.RunTimedOut).
		On("publication names", deletedSkills.Deliver, outbox.SkillDeleted).
		Ignore("progress announcements: a run that is still moving is read from its own row by the UI, and no worker-side reaction is owed",
			outbox.RunQueued, outbox.RunProvisioning, outbox.RunPreparing,
			outbox.RunRunning, outbox.RunEvaluating).
		Ignore("cleanup outcome is already recorded on the run row and alerted on through metrics; nothing in this process reacts to it",
			outbox.RunCleanupCleaned, outbox.RunCleanupFailed).
		Ignore("run bookkeeping: provider, attempts, object grants and cancel requests are read from the run's own rows, and nothing reacts to them yet",
			outbox.RunCancelRequested, outbox.RunProviderAssigned, outbox.RunAttemptStarted,
			outbox.RunAttemptDispatched, outbox.RunAttemptFinished, outbox.RunObjectGrantsRecorded).
		Ignore("evaluation facts: no aggregate reacts to them yet, and every reader answers from the evaluation's own rows",
			outbox.EvaluationStarted, outbox.EvaluationSuperseded, outbox.EvaluationCompleted,
			outbox.EvaluationFailed, outbox.EvaluationFeedbackRecorded, outbox.EvaluationSuggestionDecided,
			outbox.EvaluationSuggestionsApplied).
		Ignore("skill facts: no aggregate reacts to them yet, and every reader answers from the skill's own rows",
			outbox.SkillTakenDown, outbox.SkillAccessRestricted, outbox.SkillAccessRestrictionLifted,
			outbox.SkillRedistributionSet, outbox.SkillCategorized,
			outbox.SkillCreated, outbox.SkillDescribed, outbox.SkillCurationSet, outbox.SkillRestored)
}

func newCreationServices(
	pool *pgxpool.Pool, deps Deps, registrySvc *registry.Service,
) (*creation.Service, *ingest.Service, *catalog.Service) {
	creationSvc := &creation.Service{Pool: pool, Limits: deps.CreationLimits, LLM: creation.ModelOrNone(deps.LLM)}
	creationVersions := &ingest.Service{Pool: pool, Store: deps.Store, References: registrySvc}
	creationSearch := &catalog.Service{Pool: pool, LLM: catalog.ModelOrNone(deps.LLM), CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	wireCreationReads(creationSvc, creationVersions, creationSearch)
	wireCreationGateway(creationSvc, deps.Gateway)
	wireCreationFetch(creationSvc)
	return creationSvc, creationVersions, creationSearch
}

func addDomainWorkers(set *Set, workers *river.Workers) {
	addWorker(set, workers, &CreationStepWorker{Svc: set.Creation})
	addWorker(set, workers, &CreationExpiryWorker{Svc: set.Creation})

	addWorker(set, workers, &RunExecuteWorker{Runs: set.Runs})
	addWorker(set, workers, &RunCleanupWorker{Runs: set.Runs})
	addWorker(set, workers, &RunOrphanScanWorker{Runs: set.Runs})
	addWorker(set, workers, &RunSuperviseWorker{Runs: set.Runs})
	addWorker(set, workers, &EvaluationExecuteWorker{Svc: set.Evaluations})
	addWorker(set, workers, &EvaluationRecoveryWorker{Svc: set.Evaluations})
	addWorker(set, workers, &UnevaluatedRunSweepWorker{Consumer: set.RunEvents})
	addWorker(set, workers, &SuggestionsAppliedWorker{Svc: set.Evaluations})
	addWorker(set, workers, &EvaluationBatchAdvanceWorker{Svc: set.Batches})
}

func newObjectReconciler(
	pool *pgxpool.Pool, store *objstore.Client, downloads *packaging.Service, testlabSvc *testlab.Service,
) *objreconcile.Service {
	return &objreconcile.Service{
		Pool: pool, Store: store,
		ListExpiredArtifacts:       wiring.ReconcileCandidates(downloads.ExpiredReconcileCandidates),
		ListDownloadIntents:        wiring.ReconcileCandidates(downloads.DownloadCleanupIntentCandidates),
		ListClaimedArtifacts:       wiring.ReconcileCandidates(downloads.ClaimedReconcileCandidates),
		ListClaimedDatasets:        wiring.ReconcileCandidates(testlabSvc.ClaimedReconcileCandidates),
		RecordArtifactPurged:       downloads.MarkArtifactPurged,
		RecordDownloadIntentPurged: downloads.MarkDownloadCleanupIntentPurged,
		RecordDatasetLost:          testlabSvc.MarkDatasetObjectLost,
		GuardArtifactRemoval:       downloads.GuardArtifactRemoval,
	}
}

func periodicJobs(set *Set, deps Deps, outboxWorker *outbox.Worker) []*river.PeriodicJob {
	var periodic []*river.PeriodicJob
	scheduleAt := func(args river.JobArgs, when river.PeriodicSchedule, runOnStart bool) {
		set.Scheduled[args.Kind()] = runOnStart
		var opts *river.PeriodicJobOpts
		if runOnStart {
			opts = &river.PeriodicJobOpts{RunOnStart: true}
		}
		insert := periodicInsert(args)
		periodic = append(periodic, river.NewPeriodicJob(when,
			func() (river.JobArgs, *river.InsertOpts) { return args, insert }, opts))
	}
	schedule := func(args river.JobArgs, every time.Duration, runOnStart bool) {
		scheduleAt(args, river.PeriodicInterval(every), runOnStart)
	}
	schedule(EvaluationRecoveryArgs{}, eval.RecoveryInterval, true)
	schedule(UnevaluatedRunSweepArgs{}, eval.UnevaluatedSweepInterval, false)
	schedule(batch.BatchAdvanceArgs{}, batch.BatchAdvanceInterval, true)
	schedule(RunSuperviseArgs{}, run.SuperviseInterval, true)
	if deps.CreationLimits.Valid() {
		schedule(wiring.CreationExpiryArgs{}, time.Minute, true)
	}
	schedule(RunOrphanScanArgs{}, run.OrphanScanInterval, true)

	schedule(outbox.PublishArgs{}, outboxWorker.Interval(), true)

	schedule(objreconcile.Args{}, objreconcile.Interval, false)

	for _, kind := range credit.AllStatisticKinds() {
		schedule(wiring.NewCreditRecomputeArgs(credit.RecomputeArgs{StatKind: kind, WindowSeconds: int64(creditStatWindow / time.Second)}), creditRecomputeInterval, false)
	}

	schedule(PartitionCreateArgs{}, PartitionCreateInterval, true)
	schedule(CapacitySampleArgs{}, capacity.SampleInterval, true)

	schedule(EnrichmentBackfillArgs{}, EnrichmentBackfillInterval, false)
	schedule(ProposalExpiryArgs{}, proposalExpiryInterval, true)
	if agentRunsAvailable(deps) {
		for _, def := range operations.Definitions() {
			scheduleAt(PlatformAgentRunArgs{Agent: def.Name}, dailyAt{hour: platformAgentHourUTC}, false)
		}
	}
	return periodic
}

func addGaugePublishers(set *Set, workers *river.Workers, outboxWorker *outbox.Worker, backlogs map[string]backlogOldest) {
	observer := &BacklogObserveWorker{Backlogs: backlogs}
	addWorker(set, workers, observer)
	set.Gauges = []func(context.Context) error{set.Runs.PublishGauges, outboxWorker.PublishGauge, set.Objects.PublishGauge, observer.Observe}
}

func addCapacityObserver(set *Set, workers *river.Workers, store capacity.Store) {
	addWorker(set, workers, &CapacitySampleWorker{Store: store})
	set.Gauges = append(set.Gauges, store.PublishGauges, jobruns.PublishGauges(store.Pool))
}

func connectQueue(set *Set, client *river.Client[pgx.Tx]) {
	set.Queue = client
	set.Creation.Insert = wiring.NewCreationQueue(client)

	set.Runs.Queue = wiring.NewRunQueue(client)
	set.RunEvents.Enqueue = wiring.NewEvaluationEnqueue(client)
	set.BatchEvents.Enqueue = wiring.NewBatchAdvanceEnqueue(client)

	set.SkillVersions.Enqueue = wiring.NewSuggestionsAppliedEnqueue(client)
}

var periodicQueue = map[string]string{
	RunSuperviseArgs{}.Kind():       wiring.QueueRuns,
	RunOrphanScanArgs{}.Kind():      wiring.QueueRuns,
	EnrichmentBackfillArgs{}.Kind(): wiring.QueueModel,
}

func periodicInsert(args river.JobArgs) *river.InsertOpts {
	return &river.InsertOpts{Queue: periodicQueue[args.Kind()]}
}

func riverConfig(workers *river.Workers, periodic []*river.PeriodicJob, pollOnly bool) *river.Config {
	return &river.Config{
		Workers:      workers,
		Queues:       wiring.Queues,
		PeriodicJobs: periodic,
		PollOnly:     pollOnly,
	}
}

func addWorker[T river.JobArgs](set *Set, workers *river.Workers, worker river.Worker[T]) {
	var args T
	set.WorkerKinds[args.Kind()] = true
	river.AddWorker(workers, worker)
}
