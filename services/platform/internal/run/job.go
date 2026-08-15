package run

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

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

// Worker drives a queued run through the state machine. Go is the only queue
// consumer (iron rule 7).
type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

// errNoProvider is what every run currently ends with. There is no sandbox
// provider yet — SelfHostedProvider is RUN-003/SBX-001 — and the deliberate
// choice here is to fail the run honestly rather than report a success nobody
// executed. When a provider lands, this is the branch it replaces.
var errNoProvider = errors.New("no sandbox provider is configured")

// Work advances a queued run as far as the platform can take it today:
//
//	queued → provisioning → preparing → failed (no provider)
//
// It is idempotent under River's at-least-once delivery. A redelivered job finds
// the run somewhere other than queued and returns without touching it; a
// transition that loses the race returns ErrConflict, which is likewise not an
// error worth retrying.
//
// It returns nil even when the run fails. The job succeeded — it correctly
// recorded a failed run. Returning an error would make River retry, and retry
// policy (which failure classes are retryable, and how many times) is RUN-006.
func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err // malformed payload: nothing to look up, let River discard it
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}

	run, err := w.Svc.Get(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		slog.Warn("run job for unknown run", "run_id", job.Args.RunID)
		return nil
	}
	if err != nil {
		return err
	}
	if run.Status != gen.RunStatusQueued {
		slog.Info("run job skipped, run already moved", "run_id", job.Args.RunID, "status", run.Status)
		return nil
	}

	// RUN-004: cancelling a queued run is the one case the control plane can
	// honour on its own — there is no sandbox to stop yet. Cancellation of a run
	// that has already reached a provider is RUN-006.
	if run.CancelRequestedAt.Valid {
		return w.advance(ctx, run, pgtype.UUID{}, gen.RunStatusCancelled, "cancelled before dispatch")
	}

	// One attempt row per dispatch, so a future retry adds a mapping instead of
	// overwriting this one (RUN-003).
	attempt, err := w.Svc.queries().CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
		ID: runID, WorkspaceID: workspaceID, Provider: providerUnassigned,
	})
	if err != nil {
		return err
	}

	// Provider selection and capability matching belong here, before provisioning
	// (RUN-005). Until then every run takes the same path to the same honest end.
	for _, step := range []struct {
		to     gen.RunStatus
		reason string
	}{
		{gen.RunStatusProvisioning, "dispatching to provider"},
		{gen.RunStatusPreparing, "provider acknowledged"},
	} {
		if err := w.advance(ctx, run, attempt.ID, step.to, step.reason); err != nil {
			return err
		}
		run.Status = step.to
	}

	// The provider call goes here. It does not exist, so say so and stop.
	if _, err := w.Svc.queries().FinishRunAttempt(ctx, gen.FinishRunAttemptParams{
		ID:          attempt.ID,
		WorkspaceID: workspaceID,
		// RunError.class from contracts/openapi/sandbox-provider.yaml.
		ErrorClass:   ptr("provision"),
		ErrorMessage: ptr(errNoProvider.Error()),
	}); err != nil {
		return err
	}
	// TODO(RUN-007): a terminal run must then go through cleanup. cleanup_status
	// stays 'pending' here because there is nothing provisioned to tear down.
	return w.advance(ctx, run, attempt.ID, gen.RunStatusFailed, errNoProvider.Error())
}

// advance applies one transition and swallows the "something else got there
// first" case, which is expected under at-least-once delivery, not a failure.
func (w *Worker) advance(ctx context.Context, run gen.Run, attemptID pgtype.UUID, to gen.RunStatus, reason string) error {
	_, err := w.Svc.Transition(ctx, TransitionParams{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		AttemptID:   attemptID,
		From:        run.Status,
		To:          to,
		Reason:      reason,
		// No actor: the platform moved this run, not a user.
	})
	if errors.Is(err, ErrConflict) {
		slog.Info("run transition superseded", "run_id", uuidString(run.ID), "to", to)
		return nil
	}
	return err
}

func ptr[T any](v T) *T { return &v }
