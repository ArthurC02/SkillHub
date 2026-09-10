package run

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
	SuperviseInterval = 30 * time.Second

	superviseBatch = 200
)

type SuperviseArgs struct{}

func (SuperviseArgs) Kind() string { return "run_supervise" }

type SuperviseWorker struct {
	river.WorkerDefaults[SuperviseArgs]
	Svc *Service
}

func (w *SuperviseWorker) Work(ctx context.Context, _ *river.Job[SuperviseArgs]) error {
	return w.Svc.Supervise(ctx)
}

func (s *Service) Supervise(ctx context.Context) error {
	active, err := s.queries().ListActiveRuns(ctx, superviseBatch)
	if err != nil {
		return err
	}

	var errs []error
	for _, run := range active {
		if err := s.superviseRun(ctx, run); err != nil {
			errs = append(errs, err)
		}
	}

	if backlog, err := s.queries().CountRunsNeedingCleanup(ctx); err == nil {
		metrics.CleanupBacklog.Set(float64(backlog))
	}

	stale, err := s.queries().ListRunsNeedingCleanup(ctx, superviseBatch)
	if err != nil {
		errs = append(errs, err)
	}
	for _, run := range stale {
		if s.Queue == nil {
			break
		}
		if _, err := s.Queue.Insert(ctx, CleanupArgs{
			RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID),
		}, cleanupInsertOpts()); err != nil {
			errs = append(errs, err)
		}
	}

	s.detectMaskerCanaryFailed(ctx)
	s.detectMaskingStopped(ctx)

	s.detectP02Breach(ctx)

	return errors.Join(errs...)
}

func (s *Service) superviseRun(ctx context.Context, run gen.Run) error {
	deadline := hardDeadline(run)
	if !deadline.IsZero() && time.Now().After(deadline) {

		d := &driver{svc: s, cur: run, deadline: deadline}
		err := d.finish(ctx, s.latestAttemptID(ctx, run), gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
		switch {
		case err == nil:
			slog.Warn("run timed out by the supervisor", "run_id", pgconv.UUIDString(run.ID), "status", run.Status)
		case errors.Is(err, errSuperseded):

		default:
			return err
		}
		return nil
	}

	if s.Queue == nil {
		return nil
	}

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

func (s *Service) latestAttemptID(ctx context.Context, run gen.Run) (id pgtype.UUID) {
	attempts, err := s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil || len(attempts) == 0 {
		return id
	}
	return attempts[len(attempts)-1].ID
}
