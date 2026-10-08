package run

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	SuperviseInterval = 30 * time.Second

	CleanupRescueAfter = time.Minute

	OrphanPersistsAfterRounds = 2

	superviseBatch = 200
)

func ActiveRunClaim(batch int32) gen.ListActiveRunsParams {
	return gen.ListActiveRunsParams{RecheckAfter: pgconv.Interval(SuperviseInterval), BatchSize: batch}
}

func CleanupClaim(batch int32) gen.ListRunsNeedingCleanupParams {
	return gen.ListRunsNeedingCleanupParams{
		SettledFor: pgconv.Interval(CleanupRescueAfter), RecheckAfter: pgconv.Interval(SuperviseInterval), BatchSize: batch,
	}
}

func (s *Service) Supervise(ctx context.Context) error {
	active, err := s.queries().ListActiveRuns(ctx, ActiveRunClaim(superviseBatch))
	if err != nil {
		return err
	}

	var errs []error
	policies, err := s.policiesOf(ctx, active)
	if err != nil {
		errs = append(errs, err)
		active = nil
	}
	for _, run := range active {
		if err := s.superviseRun(ctx, run, policies[run.ID.Bytes]); err != nil {
			errs = append(errs, err)
		}
	}

	_ = s.publishCleanupBacklog(ctx)

	stale, err := s.queries().ListRunsNeedingCleanup(ctx, CleanupClaim(superviseBatch))
	if err != nil {
		errs = append(errs, err)
	}
	for _, run := range stale {
		if s.Queue == nil {
			break
		}
		if err := s.Queue.Clean(ctx, RunWork{RunID: run.ID, WorkspaceID: run.WorkspaceID}); err != nil {
			errs = append(errs, err)
		}
	}

	s.detectMaskerCanaryFailed(ctx)
	s.detectMaskingStopped(ctx)

	s.detectP02Breach(ctx)

	return errors.Join(errs...)
}

func (s *Service) policiesOf(ctx context.Context, runs []gen.Run) (map[[16]byte][]byte, error) {
	if len(runs) == 0 {
		return nil, nil
	}
	ids := make([]pgtype.UUID, len(runs))
	for i, run := range runs {
		ids[i] = run.ID
	}
	rows, err := s.queries().ListRunPolicies(ctx, ids)
	if err != nil {
		return nil, err
	}
	policies := make(map[[16]byte][]byte, len(rows))
	for _, row := range rows {
		policies[row.RunID.Bytes] = row.PolicySnapshot
	}
	return policies, nil
}

func (s *Service) superviseRun(ctx context.Context, run gen.Run, policy []byte) error {
	attempts, err := s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{RunID: run.ID, WorkspaceID: run.WorkspaceID})
	if err != nil {
		return err
	}
	if clock := clockFor(run, policy, attempts); clock.expired(s.now()) {
		var lastAttemptID pgtype.UUID
		if len(attempts) > 0 {
			lastAttemptID = attempts[len(attempts)-1].ID
		}
		d := &driver{svc: s, cur: run, policy: policy, clock: clock}
		err := d.finish(ctx, lastAttemptID, gen.RunStatusTimedOut, failureTimeout, d.timeoutReason())
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

	added, err := s.Queue.Drive(ctx, RunWork{RunID: run.ID, WorkspaceID: run.WorkspaceID})
	if err != nil {
		return err
	}
	if added {
		slog.Info("re-enqueued a run with no live job", "run_id", pgconv.UUIDString(run.ID), "status", run.Status)
	}
	return nil
}

func (s *Service) PublishGauges(ctx context.Context) error {
	errs := []error{s.publishCleanupBacklog(ctx)}
	for _, provider := range s.providers().Providers {
		errs = append(errs, s.publishPersistentOrphans(ctx, provider.Name()))
	}
	s.publishHaltMetrics(ctx)
	return errors.Join(errs...)
}

func (s *Service) publishCleanupBacklog(ctx context.Context) error {
	backlog, err := s.queries().CountRunsNeedingCleanup(ctx)
	if err != nil {
		return err
	}
	metrics.CleanupBacklog.Set(float64(backlog))
	return nil
}
