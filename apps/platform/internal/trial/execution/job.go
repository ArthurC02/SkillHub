package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	// defaultMaxAttempts bounds automatic retries of one run. ADR-004 forbids
	// unbounded retries; three is one dispatch plus two recoveries from a provider
	// having a bad moment, which is where the useful part of retrying stops.
	defaultMaxAttempts = 3
	// defaultPollInterval is how often a dispatched attempt is read back. The
	// contract has no callback (iron rule 2), so this is the only way progress is
	// learned.
	defaultPollInterval = 2 * time.Second
	// jobTimeout is a backstop against a wedged worker, not the run's time limit
	// - that is the per-run wall clock in policy_snapshot, enforced by the driver and
	// by the supervisor. River's default of one minute would kill a healthy run.
	jobTimeout = 30 * time.Minute
	// reasonLimit truncates provider-supplied text before it is stored and shown.
	// The contract says these strings are masked; length is the part the platform
	// can enforce on its own.
	reasonLimit = 500
)

// liveJobStates are the River states in which a job still counts for uniqueness.
// The default set also includes `completed`, which would make a run un-re-drivable
// after any job of its kind finished - exactly what RUN-008 recovery must be able
// to do.
var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
	rivertype.JobStateRetryable,
}

// JobArgs is the queue payload for executing one run. It carries identifiers
// only: everything else is read from the database under the workspace it names,
// so a tampered payload cannot widen scope, and a job that outlives a schema
// change still resolves against current data.
type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

// Kind is the durable job name. Renaming it strands jobs already in the queue.
func (JobArgs) Kind() string { return "run_execute" }

// executeInsertOpts makes the execution job unique per run while one is live, so
// the supervisor can re-enqueue a run without racing a job that is already
// driving it (RUN-008). Every insert of this kind must use it - an insert without
// unique options carries no unique key and would not be seen as a duplicate.
func executeInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
		// River-level retries are deliberately few: a job that died mid-run is
		// resumed by re-reading provider state, and the supervisor re-enqueues
		// anything that falls through. Twenty-five attempts of a 15-minute job is
		// not a retry policy, it is a week.
		MaxAttempts: 3,
	}
}

// Worker drives a queued run through the state machine. Go is the only queue
// consumer (iron rule 7).
type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

// Timeout overrides River's one-minute default; see jobTimeout.
func (w *Worker) Timeout(*river.Job[JobArgs]) time.Duration { return jobTimeout }

// errSuperseded means something else moved the run while this job was working on
// it - the supervisor timing it out, or a redelivered job that got there first.
// Expected under at-least-once delivery, and never a reason to fail the job.
var errSuperseded = errors.New("run was moved by something else")

// Work drives one run from wherever it is to a terminal state: select a provider,
// dispatch, follow the attempt, propagate cancellation, enforce the wall clock,
// and record what happened (RUN-005, RUN-006).
//
// It returns nil for a run that ended badly. The job succeeded - it correctly
// recorded a failed run. An error return means the *job* could not do its work and
// River should try again.
func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err // malformed payload: nothing to look up, let River discard it
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Svc.Drive(ctx, workspaceID, runID)
}

// Drive takes one run from wherever it is to a terminal state. Exported because
// the queue job is only one way to ask for that: the supervisor's recovery path
// and the tests want the same work without a River job wrapped around it.
//
// A run that is already finished, or that something else is moving, is not an
// error — under at-least-once delivery both are ordinary.
func (s *Service) Drive(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	current, err := s.Get(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		slog.Warn("run job for unknown run", "run_id", pgconv.UUIDString(runID))
		return nil
	}
	if err != nil {
		return err
	}
	if IsTerminal(current.Status) {
		slog.Info("run job skipped, run already finished", "run_id", pgconv.UUIDString(runID), "status", current.Status)
		return nil
	}

	err = (&driver{svc: s, cur: current, deadline: hardDeadline(current)}).execute(ctx)
	if errors.Is(err, errSuperseded) {
		slog.Info("run driver superseded", "run_id", pgconv.UUIDString(runID))
		return nil
	}
	return err
}

// driver carries one run through one job invocation. `cur` is the run as last
// written, so every transition knows the state it is coming from without
// re-reading (the database still enforces it - see TransitionRun).
type driver struct {
	svc      *Service
	cur      gen.Run
	provider *Provider
	deadline time.Time
}

func (d *driver) execute(ctx context.Context) error {
	// RUN-004: a cancel that arrived before anything was dispatched is the one case
	// the control plane can honour on its own - there is no sandbox to stop.
	if d.cur.CancelRequestedAt.Valid && d.cur.Status == gen.RunStatusQueued {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusCancelled, failureCancelled, "cancelled before dispatch")
	}
	if d.expired() {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
	}

	live, err := d.liveAttempt(ctx)
	if err != nil {
		return err
	}
	switch {
	case live != nil:
		// RUN-008: a restart landed us back on an attempt the provider still holds.
		// Re-attaching to it is the whole point of storing provider_run_id per
		// attempt rather than per run.
		return d.follow(ctx, *live)
	case d.cur.Status == gen.RunStatusQueued || d.cur.Status == gen.RunStatusProvisioning:
		return d.dispatch(ctx)
	case d.cur.Status == gen.RunStatusEvaluating:
		// The one state past dispatch that a restart can find with nothing actually
		// wrong. settle() finishes the attempt *before* it walks the happy path, and
		// each step of that walk is its own transaction: one blip between
		// `evaluating` and `succeeded` leaves a run whose provider reported success
		// and whose artifacts are written, with no live attempt (finished_at is set)
		// for the branch below to find. Failing it would make runs.status say the
		// execution failed when it did not, which is the one question ADR-025 says
		// that column answers.
		return d.resumeEvaluating(ctx)
	default:
		return d.terminateUnresumable(ctx)
	}
}

// resumeEvaluating finishes the happy-path walk settle() began, when the attempt
// it settled is the one that reported success.
//
// "Reported success" is exactly "the attempt finished carrying no error class":
// classifyResult writes a class for every other terminal outcome, and settle takes
// all of those out through finish() without the run ever entering `evaluating`.
// Anything else here is a run that arrived in this state some way the driver did
// not put it, and gets the safe termination below rather than a walk to
// `succeeded` on evidence nobody produced.
func (d *driver) resumeEvaluating(ctx context.Context) error {
	attempts, err := d.svc.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID,
	})
	if err != nil {
		return err
	}
	if len(attempts) == 0 || attempts[len(attempts)-1].ErrorClass != nil {
		return d.terminateUnresumable(ctx)
	}
	return d.walkHappyPath(ctx, attempts[len(attempts)-1].ID)
}

// terminateUnresumable is RUN-008's other half: safe termination. The run is past
// dispatch with no attempt to resume, and the state machine has no way back to
// provisioning, so the honest outcome is a classified failure rather than a silent
// retry that would look like the original attempt.
func (d *driver) terminateUnresumable(ctx context.Context) error {
	return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePlatform,
		"no live provider attempt to resume after a restart")
}

// dispatch selects a provider and hands it the run, retrying a bounded number of
// times while the run is still in `provisioning` (RUN-005, RUN-006).
//
// The retry window closes when the run leaves `provisioning`: the state machine
// has no backward edge, so an attempt that dies after the platform has moved on
// cannot be replaced by a fresh one without rewinding a run the user is watching.
// Only provider-side failures are retried at all - a workload that ran and failed
// gets the same answer every time, and paying for it twice helps nobody.
func (d *driver) dispatch(ctx context.Context) error {
	req, policy, err := requirementsFor(d.cur)
	if err != nil {
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failurePlatform, err.Error())
	}

	// SEC-012 action ①, second entry point, and ADR-022 X-04's 「Run 停留 queued」.
	//
	// The run is left exactly where it is and the job returns clean. Not failed: a
	// halt is a property of the fleet at this instant, and the supervisor re-enqueues
	// every live run every 30 seconds (RUN-008), so dispatch resumes on its own the
	// first sweep after the switch is released. Failing them instead would turn a
	// reversible pause into a wave of runs the user has to start again.
	//
	// Fail-closed on an unreadable switch (halt.go): waiting costs a queued run some
	// minutes, and the other direction hands an untrusted workload to a fleet nobody
	// can confirm is meant to be running.
	halts := d.svc.haltsFailClosed(ctx)
	if halts.dispatchPaused(d.svc.providers()) {
		slog.Warn("dispatch paused; leaving the run queued",
			"run_id", pgconv.UUIDString(d.cur.ID), "status", d.cur.Status)
		return nil
	}

	// SEC-012 action ②: a drained node is not offered work. Its running sandboxes
	// are left to finish — that is what drain means, and destroying them would be
	// the opposite of 「保留現場」 for the incident case.
	provider, capability, profile, err := d.svc.providers().SelectExcluding(ctx, req, halts.byTarget)
	if err != nil {
		// Nothing was created, so there is nothing to clean up and no attempt to
		// record: the run is refused before dispatch, with the reason the user gets
		// to read (ADR-004).
		return d.finish(ctx, pgtype.UUID{}, gen.RunStatusFailed, failureNoProvider, err.Error())
	}
	d.provider = provider

	pinned, err := d.svc.pinProvider(ctx, d.cur, provider, capability, profile)
	if err != nil {
		return err
	}
	d.cur = pinned

	if d.cur.Status == gen.RunStatusQueued {
		if err := d.advance(ctx, pgtype.UUID{}, gen.RunStatusProvisioning, "provider selected: "+provider.Name); err != nil {
			return err
		}
	}

	var (
		lastErr       error
		lastAttemptID pgtype.UUID
	)
	for i := 0; i < d.svc.maxAttempts(); i++ {
		if d.expired() {
			return d.finish(ctx, lastAttemptID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		}
		if cancelled, err := d.cancelRequested(ctx); err != nil {
			return err
		} else if cancelled {
			return d.finish(ctx, lastAttemptID, gen.RunStatusCancelled, failureCancelled, "cancelled while dispatching")
		}

		// One attempt row per dispatch, so a retry adds a mapping instead of
		// overwriting the previous one (RUN-003).
		attempt, err := d.svc.queries().CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
			ID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID, Provider: provider.Name,
		})
		if err != nil {
			return err
		}
		lastAttemptID = attempt.ID

		request, err := d.svc.buildRunRequest(ctx, d.cur, attempt, profile, policy)
		if err != nil {
			d.failAttempt(ctx, attempt, errClassProvision, err.Error())
			return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform, err.Error())
		}

		pr, err := provider.CreateRun(ctx, request)
		if err != nil {
			lastErr = err
			d.failAttempt(ctx, attempt, dispatchErrorClass(err), err.Error())
			if !retryable(err) {
				break
			}
			slog.Warn("run dispatch failed, retrying with a new attempt",
				"run_id", pgconv.UUIDString(d.cur.ID), "attempt", attempt.AttemptNumber, "error", err)
			continue
		}

		attempt, err = d.svc.queries().SetAttemptProviderRunID(ctx, gen.SetAttemptProviderRunIDParams{
			ID: attempt.ID, WorkspaceID: d.cur.WorkspaceID, ProviderRunID: &pr.ProviderRunID,
		})
		if err != nil {
			// The sandbox exists but the platform failed to record the handle, so
			// nothing will ever clean it up by attempt: release it now rather than
			// leave it for the orphan scan to find five minutes from now.
			if destroyErr := provider.Destroy(ctx, pr.ProviderRunID); destroyErr != nil {
				// Not lost: isOrphan reclaims an attempt whose handle was never
				// recorded once the sandbox is past orphanGrace, so the ceiling on
				// this is a grace window plus a scan interval rather than the run's
				// whole wall clock.
				slog.Error("leaked a sandbox: its mapping could not be recorded and it could not be destroyed; the orphan scan will reclaim it",
					"run_id", pgconv.UUIDString(d.cur.ID), "error", destroyErr)
			}
			return err
		}

		// A provider that answered the dispatch and is already in a terminal
		// failure never got the workload going: still a provision failure, still
		// inside the retry window. The sandbox it left behind is released by the
		// cleanup this run's terminal transition schedules.
		if pr.State == ProviderStateFailed {
			lastErr = fmt.Errorf("provider failed during provisioning: %s", truncate(pr.StateReason))
			d.failAttempt(ctx, attempt, errClassProvision, lastErr.Error())
			continue
		}
		return d.follow(ctx, attempt)
	}

	message := "dispatch failed"
	if lastErr != nil {
		message = lastErr.Error()
	}
	return d.finish(ctx, lastAttemptID, gen.RunStatusFailed, failureProvider, message)
}

// follow polls one dispatched attempt to its provider-terminal state, mapping the
// provider's lifecycle onto the platform's on the way (RUN-006).
func (d *driver) follow(ctx context.Context, attempt gen.RunAttempt) error {
	provider := d.provider
	if provider == nil || provider.Name != attempt.Provider {
		provider = d.svc.providers().Lookup(attempt.Provider)
	}
	if provider == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform,
			"provider "+attempt.Provider+" is no longer configured")
	}
	d.provider = provider
	if attempt.ProviderRunID == nil {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failurePlatform, "attempt has no provider handle")
	}
	handle := *attempt.ProviderRunID

	cancelSent := false
	for {
		pr, err := provider.GetRun(ctx, handle)
		switch {
		case err == nil:
			if err := d.mapState(ctx, attempt, pr); err != nil {
				return err
			}
			if pr.State.Terminal() {
				return d.settle(ctx, attempt, pr)
			}
		case !retryable(err):
			d.failAttempt(ctx, attempt, errClassExecution, err.Error())
			return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failureProvider, err.Error())
		default:
			// Transient. Keep polling: a provider that is briefly unreachable has
			// not lost the run, and the wall clock below still bounds the wait.
			slog.Warn("provider poll failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
		}

		if !cancelSent {
			cancelled, err := d.cancelRequested(ctx)
			if err != nil {
				return err
			}
			if cancelled {
				// Ask and keep polling: the contract answers 202 and the workload is
				// only actually down when a later read says so. Reporting `cancelled`
				// here would be a lie about a live sandbox.
				if _, err := provider.Cancel(ctx, handle); err != nil {
					slog.Warn("provider cancel failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
				} else {
					cancelSent = true
				}
			}
		}

		if d.expired() {
			// The platform-side hard wall clock. The provider is supposed to enforce
			// its own; this is the backstop for one that did not, so the workload is
			// asked to stop and the run is recorded as timed out either way. The
			// sandbox itself is released by the cleanup job.
			if _, err := provider.Cancel(ctx, handle); err != nil {
				slog.Warn("provider cancel on timeout failed", "run_id", pgconv.UUIDString(d.cur.ID), "error", err)
			}
			d.failAttempt(ctx, attempt, errClassTimeout, d.timeoutReason())
			return d.finish(ctx, attempt.ID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		}

		select {
		case <-ctx.Done():
			// Shutdown, or the job's own timeout. The attempt stays live and
			// unfinished, which is exactly what the supervisor looks for.
			return ctx.Err()
		case <-time.After(d.svc.pollInterval()):
		}
	}
}

// mapState moves the platform run to where the provider's state says it should be.
// Many-to-one on purpose: `creating` covers ADR-004's provision *and* prepare, so
// the platform's own beat between them is driven by dispatch, not by the provider.
func (d *driver) mapState(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	reason := truncate(pr.StateReason)
	switch pr.State {
	case ProviderStateCreating:
		if d.cur.Status == gen.RunStatusProvisioning {
			return d.advance(ctx, attempt.ID, gen.RunStatusPreparing, orDefault(reason, "provider is creating the sandbox"))
		}
	case ProviderStateRunning:
		if d.cur.Status == gen.RunStatusProvisioning {
			if err := d.advance(ctx, attempt.ID, gen.RunStatusPreparing, "provider acknowledged the dispatch"); err != nil {
				return err
			}
		}
		if d.cur.Status == gen.RunStatusPreparing {
			return d.advance(ctx, attempt.ID, gen.RunStatusRunning, orDefault(reason, "workload started"))
		}
	}
	return nil
}

// settle records the attempt's outcome and takes the run to its terminal state.
func (d *driver) settle(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) error {
	status, failureClass, errClass, message := classifyResult(pr)
	d.recordArtifacts(ctx, attempt, pr)
	d.failAttempt(ctx, attempt, errClass, message)

	if status != gen.RunStatusSucceeded {
		return d.finish(ctx, attempt.ID, status, failureClass, message)
	}
	return d.walkHappyPath(ctx, attempt.ID)
}

// walkHappyPath takes the run from where it is to `succeeded`, one transition at a
// time.
//
// The happy path is the only one that has to walk the machine: `succeeded` is
// reachable from `evaluating` alone, while the three unhappy terminals are
// reachable from anywhere. Which successor is the happy one is the aggregate's
// answer to give (HappyPath), and the route is settled before the first transition
// is applied - a walk that cannot arrive is an error the job returns, not laps
// around a loop writing to the database.
//
// Each step commits on its own, which is why resumeEvaluating exists: a walk that
// is interrupted halfway is resumed from wherever it got to, not failed.
func (d *driver) walkHappyPath(ctx context.Context, attemptID pgtype.UUID) error {
	path, err := HappyPath(d.cur.Status)
	if err != nil {
		return err
	}
	for _, next := range path {
		if err := d.advance(ctx, attemptID, next, successReason(next)); err != nil {
			return err
		}
	}
	return nil
}

// successReason explains each step of the happy path.
//
// The last one used to carry a TODO saying evaluation would run here and decide
// `succeeded` versus `failed`. ADR-025 overturned that: `runs.status` answers
// "what happened while this executed" and `evaluations.overall` answers "was the
// task achieved", and an evaluation never writes back to the run. So the terminal
// reason is execution language and nothing more - the task verdict is a separate
// resource, enqueued by Transition and read at GET /runs/{id}/evaluation.
func successReason(to gen.RunStatus) string {
	switch to {
	case gen.RunStatusPreparing:
		return "provider acknowledged the dispatch"
	case gen.RunStatusRunning:
		return "workload started"
	case gen.RunStatusEvaluating:
		return "workload completed; collecting the result"
	default:
		return "workload ran to its own end and reported success; " +
			"whether the task was achieved is a separate judgement (see this run's evaluation)"
	}
}

// classifyResult maps one terminal ProviderRun onto a platform terminal state, the
// platform's failure vocabulary, and the provider's own RunError class.
//
// The distinction that matters is `completed` versus `failed`: a workload that ran
// correctly and reported failure is the skill's problem (workload_error, never
// retried), while a provider that could not carry the attempt is ours
// (provider_error, retryable inside the dispatch window).
func classifyResult(pr ProviderRun) (status gen.RunStatus, failureClass, errClass, message string) {
	if pr.Result == nil {
		return gen.RunStatusFailed, failureProvider, errClassProvision,
			"provider reported a terminal state with no result"
	}
	errClass, message = "", truncate(pr.StateReason)
	if pr.Result.Error != nil {
		errClass, message = pr.Result.Error.Class, truncate(pr.Result.Error.Message)
	}

	switch {
	case pr.State == ProviderStateCancelled || pr.Result.Status == "cancelled":
		return gen.RunStatusCancelled, failureCancelled,
			orDefault(errClass, errClassCancelled), orDefault(message, "stopped at the user's request")
	case pr.Result.Status == "timed_out":
		return gen.RunStatusTimedOut, failureTimeout,
			orDefault(errClass, errClassTimeout), orDefault(message, "the provider stopped the workload at its wall clock limit")
	case pr.State == ProviderStateCompleted && pr.Result.Status == "succeeded":
		return gen.RunStatusSucceeded, "", "", ""
	case pr.State == ProviderStateCompleted:
		return gen.RunStatusFailed, failureWorkload,
			orDefault(errClass, errClassExecution), orDefault(message, "the workload reported failure")
	default:
		return gen.RunStatusFailed, failureProvider,
			orDefault(errClass, errClassProvision), orDefault(message, "the provider could not carry the attempt")
	}
}

// dispatchErrorClass names a failed POST /runs in the contract's own vocabulary.
// A 422 is the provider refusing the request itself, which is a capability
// mismatch however sure the scheduler was a moment ago.
func dispatchErrorClass(err error) string {
	if pe, ok := errors.AsType[*providerError](err); ok {
		if pe.Class != "" {
			return pe.Class
		}
		if pe.Status == 422 {
			return errClassCapabilityMismatch
		}
	}
	return errClassProvision
}

// --- state plumbing ----------------------------------------------------------

// advance applies one transition and keeps `cur` in step with it.
func (d *driver) advance(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, reason string) error {
	return d.transition(ctx, attemptID, to, "", reason)
}

// finish applies the terminal transition, with the classification that explains it.
func (d *driver) finish(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass, reason string) error {
	return d.transition(ctx, attemptID, to, failureClass, reason)
}

func (d *driver) transition(ctx context.Context, attemptID pgtype.UUID, to gen.RunStatus, failureClass, reason string) error {
	run, err := d.svc.Transition(ctx, TransitionParams{
		WorkspaceID:  d.cur.WorkspaceID,
		RunID:        d.cur.ID,
		AttemptID:    attemptID,
		From:         d.cur.Status,
		To:           to,
		Reason:       truncate(reason),
		FailureClass: failureClass,
		// No actor: the platform moved this run, not a user.
	})
	if errors.Is(err, ErrConflict) {
		return errSuperseded
	}
	if err != nil {
		return err
	}
	d.cur = run
	return nil
}

// recordArtifacts stores the manifest the attempt reported (RUN-002's collected
// output, EVAL-001's evidence). Manifest only: names, sizes and hashes, with the
// bytes left in the archive the attempt's write grant addressed.
//
// Best effort, like failAttempt: the run's terminal state is the fact that
// matters, and a manifest that could not be written must not stop it. What it
// costs is an evaluation that reports no output files for a run that had some -
// which is why the failure is logged rather than swallowed.
func (d *driver) recordArtifacts(ctx context.Context, attempt gen.RunAttempt, pr ProviderRun) {
	if pr.Result == nil || len(pr.Result.Artifacts) == 0 {
		return
	}
	archiveKey := artifactObjectKey(pgconv.UUIDString(d.cur.ID), pgconv.UUIDString(attempt.ID))
	for _, a := range pr.Result.Artifacts {
		if a.FileName == "" {
			continue
		}
		key := a.ObjectKey
		if key == "" {
			key = archiveKey
		}
		contentType := a.ContentType
		if contentType == "" {
			// From the manifest's own magic-byte sniff, not from the file name.
			// Unknown is a real answer and is stored as one.
			contentType = "application/octet-stream"
		}
		if _, err := d.svc.queries().InsertRunArtifact(ctx, gen.InsertRunArtifactParams{
			WorkspaceID: d.cur.WorkspaceID, RunID: d.cur.ID,
			FileName: a.FileName, ContentType: contentType, SizeBytes: a.SizeBytes,
			ContentHash: a.ContentHash, ObjectKey: key,
		}); err != nil {
			slog.Warn("could not record an artifact manifest row",
				"run_id", pgconv.UUIDString(d.cur.ID), "file_name", a.FileName, "error", err)
		}
	}
}

// failAttempt records the attempt's own outcome. Best effort by design: the run's
// terminal state is the fact that matters, and losing the attempt annotation must
// not stop it from being written.
func (d *driver) failAttempt(ctx context.Context, attempt gen.RunAttempt, errClass, message string) {
	var classPtr, messagePtr *string
	if errClass != "" {
		classPtr = &errClass
	}
	if message != "" {
		truncated := truncate(message)
		messagePtr = &truncated
	}
	if _, err := d.svc.queries().FinishRunAttempt(ctx, gen.FinishRunAttemptParams{
		ID: attempt.ID, WorkspaceID: attempt.WorkspaceID,
		ErrorClass: classPtr, ErrorMessage: messagePtr,
	}); err != nil {
		slog.Warn("could not record attempt outcome", "run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
	}
}

// liveAttempt is the attempt a provider may still be holding: dispatched (it has a
// handle) and not yet finished. At most one exists, because an attempt is finished
// before the next is created.
func (d *driver) liveAttempt(ctx context.Context) (*gen.RunAttempt, error) {
	attempts, err := d.svc.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: d.cur.ID, WorkspaceID: d.cur.WorkspaceID,
	})
	if err != nil {
		return nil, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].ProviderRunID != nil && !attempts[i].FinishedAt.Valid {
			return &attempts[i], nil
		}
	}
	return nil, nil
}

// cancelRequested re-reads the intent the API recorded. Read every poll rather
// than once, because a user cancels *during* a run, which is the whole point.
func (d *driver) cancelRequested(ctx context.Context) (bool, error) {
	fresh, err := d.svc.Get(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return false, err
	}
	if IsTerminal(fresh.Status) {
		return false, errSuperseded
	}
	d.cur.CancelRequestedAt = fresh.CancelRequestedAt
	return fresh.CancelRequestedAt.Valid, nil
}

// hardDeadline is when the run must be over, wall clock, counted from creation:
// PDM-005 5.2's limit covers queue wait as well as execution, so a run can time
// out before a sandbox ever exists.
func hardDeadline(run gen.Run) time.Time {
	seconds := DefaultResourceLimits().WallClockHardSeconds
	var policy policySnapshot
	if err := json.Unmarshal(run.PolicySnapshot, &policy); err == nil && policy.ResourceLimits.WallClockHardSeconds > 0 {
		seconds = policy.ResourceLimits.WallClockHardSeconds
	}
	if !run.CreatedAt.Valid {
		return time.Time{}
	}
	return run.CreatedAt.Time.Add(time.Duration(seconds) * time.Second)
}

func (d *driver) expired() bool {
	return !d.deadline.IsZero() && time.Now().After(d.deadline)
}

func (d *driver) timeoutReason() string {
	return fmt.Sprintf("exceeded the hard wall clock limit; deadline was %s", d.deadline.UTC().Format(time.RFC3339))
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// truncate bounds a status reason at reasonLimit bytes without splitting a rune.
//
// The cut is by bytes because the column's limit is, but a bare s[:n] can land
// mid-sequence, and the result is not a Go problem - it is a Postgres one. Both
// destinations (runs.status_reason and run_attempts.error_message) are text
// columns in a UTF8 database, which refuse invalid byte sequences outright. The
// write that fails is the one carrying the terminal state, so a Traditional
// Chinese failure message long enough to be cut would leave the Run stuck in a
// non-terminal state forever. Same fix as design.truncate.
func truncate(s string) string {
	if len(s) <= reasonLimit {
		return s
	}
	return strings.ToValidUTF8(s[:reasonLimit], "") + "..."
}
