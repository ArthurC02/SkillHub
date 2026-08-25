package run

// RUN-008: the control plane reconciling itself against its own database.
//
// One periodic job does three things that are the same scan wearing different
// hats: re-drive runs whose job died with the process, stop runs that outlived
// their wall clock, and re-enqueue cleanups that never finished. Splitting them
// into three jobs would mean three leader-elected timers reading the same two
// tables.
//
// It runs on start as well as on its interval (RunOnStart in cmd/worker), which is
// what makes it the restart-recovery path rather than only a watchdog.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	// SuperviseInterval is how often the supervisor sweeps. Short enough that a
	// hard timeout is enforced promptly, long enough that it is not a poll loop on
	// the runs table.
	SuperviseInterval = 30 * time.Second
	// superviseBatch bounds one sweep. The live set is bounded by provider slots,
	// so this is a safety rail rather than pagination.
	superviseBatch = 200
)

// SuperviseArgs carries nothing: the work is "whatever the database says is
// outstanding", and putting a cursor in the payload would only let a stale job
// resume a scan that has moved on.
type SuperviseArgs struct{}

func (SuperviseArgs) Kind() string { return "run_supervise" }

// SuperviseWorker re-drives, times out and re-cleans.
type SuperviseWorker struct {
	river.WorkerDefaults[SuperviseArgs]
	Svc *Service
}

func (w *SuperviseWorker) Work(ctx context.Context, _ *river.Job[SuperviseArgs]) error {
	return w.Svc.Supervise(ctx)
}

// Supervise is one sweep. Exported so cmd/worker and tests can run it directly
// rather than waiting for a timer.
func (s *Service) Supervise(ctx context.Context) error {
	active, err := s.queries().ListActiveRuns(ctx, superviseBatch)
	if err != nil {
		return err
	}
	for _, run := range active {
		if err := s.superviseRun(ctx, run); err != nil {
			return err
		}
	}

	// O11Y-003: the cleanup backlog gauge is published from here rather than from
	// the cleanup job, because a backlog is a thing that persists between passes -
	// counting it where the sweep happens is the only place it is ever true.
	if backlog, err := s.queries().CountRunsNeedingCleanup(ctx); err == nil {
		metrics.CleanupBacklog.Set(float64(backlog))
	}

	stale, err := s.queries().ListRunsNeedingCleanup(ctx, superviseBatch)
	if err != nil {
		return err
	}
	for _, run := range stale {
		if s.Queue == nil {
			break
		}
		if _, err := s.Queue.Insert(ctx, CleanupArgs{
			RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID),
		}, cleanupInsertOpts()); err != nil {
			return err
		}
	}

	// 03:SEC-012, the P1 criterion of 02:SEC-010 that this process can conclude on
	// its own, read two ways. It rides the sweep rather than getting a timer of its
	// own for the reason the file header gives: another leader-elected timer reading
	// the same tables buys nothing. Neither ever returns an error, same as
	// EvaluateOrphanThresholds at the end of the orphan scan — a detector must not
	// be able to fail the work it rides on.
	//
	// The canary asks the masker directly and needs no traffic; the traffic reading
	// asks whether a whole window of real events went unredacted, which is the only
	// way the ingestion path skipping the masker altogether becomes visible. Neither
	// sees the other's failure — halt.go's detectMaskerCanaryFailed has the long version.
	//
	// The other criterion (Reconciler 停擺) is deliberately NOT here: this sweep is
	// a River periodic job, and a stopped worker would take its own watchdog with
	// it. That one lives in cmd/api (halt.go's WatchReconciler).
	s.detectMaskerCanaryFailed(ctx)
	s.detectMaskingStopped(ctx)
	return nil
}

// superviseRun decides what one still-moving run needs.
func (s *Service) superviseRun(ctx context.Context, run gen.Run) error {
	deadline := hardDeadline(run)
	if !deadline.IsZero() && time.Now().After(deadline) {
		// The wall clock ran out and no job is enforcing it — either none is
		// running, or the one that is has wedged. Recording the timeout here is what
		// keeps a lost worker from leaving a run "running" forever; the terminal
		// transition enqueues the cleanup that releases the sandbox.
		d := &driver{svc: s, cur: run, deadline: deadline}
		err := d.finish(ctx, s.latestAttemptID(ctx, run), gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		switch {
		case err == nil:
			slog.Warn("run timed out by the supervisor", "run_id", pgconv.UUIDString(run.ID), "status", run.Status)
		case errors.Is(err, errSuperseded):
			// A driver got there first. That is the good outcome.
		default:
			return err
		}
		return nil
	}

	if s.Queue == nil {
		return nil
	}
	// Re-enqueue. The insert is unique per run over the live job states, so this is
	// a no-op whenever a job is already driving the run — which is the common case,
	// and why this can run every 30 seconds without doing anything.
	res, err := s.Queue.Insert(ctx, JobArgs{
		RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID),
	}, executeInsertOpts())
	if err != nil {
		return err
	}
	if !res.UniqueSkippedAsDuplicate {
		slog.Info("re-enqueued a run with no live job", "run_id", pgconv.UUIDString(run.ID), "status", run.Status)
	}
	return nil
}

// latestAttemptID is the attempt a supervisor-decided terminal state belongs to,
// or the zero UUID when the run never got that far. Best effort: failing to name
// the attempt must not stop the run from being timed out.
func (s *Service) latestAttemptID(ctx context.Context, run gen.Run) (id pgtype.UUID) {
	attempts, err := s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil || len(attempts) == 0 {
		return id
	}
	return attempts[len(attempts)-1].ID
}
