package credit

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// RecomputeArgs is ADR-068 decision 9's fixed-time half: one daily River job
// per cost-event kind, recomputing that kind's rolling-window statistics.
// Scheduling (which kinds run, and river.PeriodicJob's daily cadence) is the
// composition root's job — see this batch's report for exactly where to add
// it (entrypoint/worker/worker.go); this package only supplies the worker.
type RecomputeArgs struct {
	StatKind      string `json:"stat_kind"`
	WindowSeconds int64  `json:"window_seconds"`
}

func (RecomputeArgs) Kind() string                 { return "credit_recompute_statistics" }
func (RecomputeArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

// RecomputeWorker runs RecomputeArgs jobs against an injected Service.
type RecomputeWorker struct {
	river.WorkerDefaults[RecomputeArgs]
	Svc *Service
}

func (w *RecomputeWorker) Work(ctx context.Context, j *river.Job[RecomputeArgs]) error {
	window := time.Duration(j.Args.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}
	_, err := w.Svc.RecomputeStatistics(ctx, j.Args.StatKind, window)
	return err
}
