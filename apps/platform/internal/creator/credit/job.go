package credit

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

type RecomputeArgs struct {
	StatKind      string `json:"stat_kind"`
	WindowSeconds int64  `json:"window_seconds"`
}

func (RecomputeArgs) Kind() string                 { return "credit_recompute_statistics" }
func (RecomputeArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

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
