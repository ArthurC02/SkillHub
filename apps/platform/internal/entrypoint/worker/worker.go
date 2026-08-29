// Package worker is cmd/worker's composition root (ADR-032 §5 實作註記):
// BuildWorkers wires the process's object graph — run.Service, eval.Service and
// the rest of the River worker set — not apiserver.NewApp, which wires the
// API's graph instead; the two deployment units share no object. BuildWorkers
// does no I/O, which is what lets worker_test.go check the wiring without a
// database — the check that was missing when this file forgot to set
// run.Service.Queue and every run finished un-cleaned. cmd/worker's main()
// keeps what is genuinely the process: reading the environment, starting the
// queue client and shutting it down.
package worker

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// Deps is everything the environment supplies to this process's wiring.
// Deployment inputs only, in the same spirit as apiserver.Config: reading them is
// main's job, and building a domain Service out of them is BuildWorkers'.
type Deps struct {
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
	// PollOnly starts the River client without issuing LISTEN, so it never asks
	// the pool for a second connection just to notify. The zero value (false)
	// is cmd/worker's own behaviour and must stay that way — cmd/worker never
	// sets this field, so its LISTEN-based low-latency dispatch is unchanged.
	// The one caller that sets it true is cmd/api's clean test mode, where a
	// single pool_max_conns=1 connection is all there is and a second LISTEN
	// connection is not available to ask for (ADR-060 決策 6).
	PollOnly bool
}

// Set is the wired graph this process runs. Exposed field by field rather
// than returned as a started client, because "which dependency reached which
// service" is exactly what the wiring test has to be able to look at.
type Set struct {
	Runs        *run.Service
	Evaluations *eval.Service
	Packaging   *packaging.Service
	RunEvents   *eval.RunEventConsumer
	Events      *outbox.Dispatcher
	Objects     *objreconcile.Service
	Queue       *river.Client[pgx.Tx]
	// WorkerKinds is every job kind that has a worker registered, and Scheduled
	// maps every kind this process enqueues on a timer to its RunOnStart — the
	// value, not merely the presence, because RunOnStart is the whole difference
	// between recovering at a restart and recovering an interval later.
	// Recorded while wiring because River's registry cannot be read back: a
	// periodic job whose worker was dropped fails only when the insert is
	// attempted, one interval into a deployment, and only in the log.
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

// BuildWorkers wires this process's object graph: no environment reads, no
// database round trips, no goroutines. Everything here is a struct literal, a
// registration or a back-assignment, so the failure it exists to prevent — a
// dependency nobody noticed was never set — is reachable from a test.
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
		ReadSkill: func(ctx context.Context, workspaceID, skillID pgtype.UUID) (run.SkillFacts, bool, error) {
			skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
			return run.SkillFacts{AccessRestriction: skill.AccessRestriction}, found, err
		},
		ReadVersion: func(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.VersionFacts, bool, error) {
			version, found, err := registrySvc.WorkspaceVersion(ctx, workspaceID, versionID)
			return run.VersionFacts{
				ID: version.ID, SkillID: version.SkillID, ContentHash: version.ContentHash,
				PackageObjectKey: version.PackageObjectKey,
			}, found, err
		},
	}
	traceSvc := newTraceService(pool, deps.TraceSigner, set.Runs)
	set.Runs.Trace = traceSvc

	// The Trace context, injected rather than built inside eval's own methods
	// (ADR-032 §5). Same signer as the dispatcher above, so the process has one
	// Trace configuration and not two.
	set.Evaluations = &eval.Service{
		Pool: pool, Store: deps.Store,
		Trace: traceSvc, TestLab: testlabSvc,
	}
	wireEvaluationRunReaders(set.Evaluations, set.Runs)
	wireEvaluationRegistryReaders(set.Evaluations, registrySvc)
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
	set.RunEvents = &eval.RunEventConsumer{HasCurrentEvaluation: set.Evaluations.HasCurrentEvaluation}

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
	// either is left unset — see worker_test.go.
	set.Objects = &objreconcile.Service{
		Pool: pool, Store: deps.Store,
		ListExpiredArtifacts: packagingCandidates(downloads.ExpiredReconcileCandidates),
		ListClaimedArtifacts: packagingCandidates(downloads.ClaimedReconcileCandidates),
		ListClaimedDatasets:  datasetCandidates(testlabSvc.ClaimedReconcileCandidates),
		RecordArtifactPurged: downloads.MarkArtifactPurged,
		RecordDatasetLost:    testlabSvc.MarkDatasetObjectLost,
	}
	addWorker(set, workers, &objreconcile.Worker{Svc: set.Objects})

	// Periodic jobs run on the elected leader only, so several worker processes
	// do not each sweep. RunOnStart is what makes the supervisor the restart
	// recovery path (RUN-008) and not merely a watchdog.
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
	schedule(run.OrphanScanArgs{}, run.OrphanScanInterval, true)
	// RunOnStart because an evaluation now waits on this drain: a restart that
	// left events in the backlog should not cost the user a full interval before
	// their finished run gets a verdict.
	schedule(outbox.PublishArgs{}, outboxWorker.Interval(), true)
	// No RunOnStart, unlike the four above: this one is not recovering from a
	// restart, and a deploy loop would otherwise re-probe every stored object on
	// every rollout. An hour late is on time here.
	schedule(objreconcile.Args{}, objreconcile.Interval, false)

	client, err := queue.New(pool, riverConfig(workers, periodic, deps.PollOnly))
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

func newTraceService(pool *pgxpool.Pool, signer *trace.Signer, runs *run.Service) *trace.Service {
	return &trace.Service{
		Pool: pool, Signer: signer,
		ReadRunState: func(ctx context.Context, workspaceID, runID pgtype.UUID) (trace.RunState, bool, error) {
			state, found, err := runs.TraceRun(ctx, workspaceID, runID)
			return trace.RunState{Status: state.Status, StatusReason: state.StatusReason}, found, err
		},
		ReadIngestRunState: func(ctx context.Context, runID pgtype.UUID) (trace.IngestRunState, bool, error) {
			state, found, err := runs.TraceIngestRun(ctx, runID)
			return trace.IngestRunState{
				ID: state.ID, WorkspaceID: state.WorkspaceID, Status: state.Status, FinishedAt: state.FinishedAt,
			}, found, err
		},
		ReadRunTransitions: func(ctx context.Context, workspaceID, runID pgtype.UUID) ([]trace.RunTransition, error) {
			rows, err := runs.TraceTransitions(ctx, workspaceID, runID)
			if err != nil {
				return nil, err
			}
			out := make([]trace.RunTransition, len(rows))
			for i, row := range rows {
				out[i] = trace.RunTransition{ToStatus: row.ToStatus, Reason: row.Reason}
			}
			return out, nil
		},
	}
}

func wireEvaluationRunReaders(service *eval.Service, runs *run.Service) {
	service.ReadRunFacts = func(ctx context.Context, workspaceID, runID pgtype.UUID) (eval.RunFacts, bool, error) {
		facts, found, err := runs.EvaluationRun(ctx, workspaceID, runID)
		return evalRunFacts(facts), found, err
	}
	service.ReadEvaluationInput = func(ctx context.Context, workspaceID, runID pgtype.UUID) (eval.EvaluationInput, bool, error) {
		input, found, err := runs.EvaluationInput(ctx, workspaceID, runID)
		artifacts := make([]eval.ArtifactFacts, len(input.Artifacts))
		for i, artifact := range input.Artifacts {
			artifacts[i] = eval.ArtifactFacts{
				FileName: artifact.FileName, ContentType: artifact.ContentType,
				SizeBytes: artifact.SizeBytes, ContentHash: artifact.ContentHash,
			}
		}
		return eval.EvaluationInput{
			Run: evalRunFacts(input.Run), Artifacts: artifacts, LatestAttempt: input.LatestAttempt,
			Absent: eval.ArtifactAbsence{
				Deleted: input.Absent.Deleted,
				Expired: input.Absent.Expired,
			},
		}, found, err
	}
}

func wireEvaluationRegistryReaders(service *eval.Service, registryService *registry.Service) {
	service.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (eval.VersionFacts, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		return eval.VersionFacts{ID: version.ID, SkillID: version.SkillID, PackageObjectKey: version.PackageObjectKey}, found, err
	}
	service.ReadLatestVersion = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (eval.VersionFacts, bool, error) {
		version, found, err := registryService.LatestVersion(ctx, workspaceID, skillID)
		return eval.VersionFacts{ID: version.ID, SkillID: version.SkillID, PackageObjectKey: version.PackageObjectKey}, found, err
	}
	service.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (eval.SkillFacts, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return eval.SkillFacts{
			ID: skill.ID, Name: skill.Name, Summary: skill.Summary, AccessRestriction: skill.AccessRestriction,
		}, found, err
	}
	service.ReadRuntimeCompatibility = func(ctx context.Context, versionID pgtype.UUID) (eval.RuntimeCompatibility, bool, error) {
		compat, found, err := registryService.RuntimeCompatibility(ctx, versionID)
		return eval.RuntimeCompatibility{
			Capability: compat.Capability, Runtime: compat.Runtime, RuntimeImage: compat.RuntimeImage,
		}, found, err
	}
}

func evalRunFacts(facts run.EvaluationRun) eval.RunFacts {
	return eval.RunFacts{
		ID: facts.ID, WorkspaceID: facts.WorkspaceID,
		SkillVersionID: facts.SkillVersionID, TestCaseSnapshotID: facts.TestCaseSnapshotID,
		Status: facts.Status, StatusReason: facts.StatusReason, RuntimeSnapshot: facts.RuntimeSnapshot,
		StartedAt: facts.StartedAt, FinishedAt: facts.FinishedAt, FailureClass: facts.FailureClass,
	}
}

// riverConfig is queue.New's argument, split out from BuildWorkers so the
// PollOnly wiring is something a test can read back (river.Client keeps no
// exported accessor for the *river.Config it was built with, so this is the
// only point after which the value stops being observable).
func riverConfig(workers *river.Workers, periodic []*river.PeriodicJob, pollOnly bool) *river.Config {
	return &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			// One queue until there is a measured reason for more. Concurrency is
			// bounded well below the per-workspace limit of 2 concurrent runs
			// (PDM-005 §5.2); real capacity planning lands with the first load test.
			river.QueueDefault: {MaxWorkers: min(runtime.NumCPU(), 4)},
		},
		PeriodicJobs: periodic,
		PollOnly:     pollOnly,
	}
}

// addWorker registers one worker and remembers the kind it claims, which is the
// only way to get that list back out: River's registry is unexported.
func addWorker[T river.JobArgs](set *Set, workers *river.Workers, worker river.Worker[T]) {
	var args T
	set.WorkerKinds[args.Kind()] = true
	river.AddWorker(workers, worker)
}
