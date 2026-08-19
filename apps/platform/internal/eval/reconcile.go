package eval

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
)

const (
	RecoveryInterval   = 5 * time.Minute
	RecoveryStaleAfter = 10 * time.Minute
)

// RecoveryArgs closes stale pending evaluations without ever invoking Judge.
// It is periodic so a recovery attempt that coincides with a database outage is
// retried after the ordinary evaluation job has exhausted its delivery attempts.
type RecoveryArgs struct{}

func (RecoveryArgs) Kind() string { return "recover_pending_evaluations" }

type RecoveryWorker struct {
	river.WorkerDefaults[RecoveryArgs]
	Svc *Service
}

func (w *RecoveryWorker) Work(ctx context.Context, _ *river.Job[RecoveryArgs]) error {
	rows, err := w.Svc.Pool.Query(ctx, `
		SELECT id, workspace_id, run_id
		FROM evaluations
		WHERE status = 'pending' AND superseded_at IS NULL
		  AND created_at < now() - make_interval(secs => $1)
		ORDER BY created_at
		LIMIT 100`, int64(RecoveryStaleAfter/time.Second))
	if err != nil {
		return err
	}
	defer rows.Close()

	type target struct{ id, workspaceID, runID pgtype.UUID }
	targets := make([]target, 0)
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.workspaceID, &t.runID); err != nil {
			return err
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range targets {
		if err := w.Svc.recoverEvaluation(ctx, t.id, t.workspaceID, t.runID); err != nil {
			return err
		}
	}
	return nil
}
