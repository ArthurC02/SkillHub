package jobruns

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Job struct {
	Name   string
	Period time.Duration
}

type Status struct {
	Job           string     `json:"job"`
	PeriodSeconds int64      `json:"period_seconds"`
	LastSucceeded *time.Time `json:"last_succeeded_at"`
	OverdueRatio  float64    `json:"overdue_ratio"`
}

func Register(ctx context.Context, pool *pgxpool.Pool, jobs []Job) error {
	names := make([]string, len(jobs))
	periods := make([]int32, len(jobs))
	for i, job := range jobs {
		names[i], periods[i] = job.Name, int32(job.Period/time.Second)
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := q.ForgetUnscheduledMaintenanceJobs(ctx, names); err != nil {
			return err
		}
		return q.RegisterMaintenanceJobs(ctx, gen.RegisterMaintenanceJobsParams{Jobs: names, PeriodSeconds: periods})
	})
}

func RecordSuccess(ctx context.Context, pool *pgxpool.Pool, job string) error {
	n, err := gen.New(pool).RecordMaintenanceJobSuccess(ctx, job)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("jobruns: %s succeeded but is not a registered maintenance job", job)
	}
	return nil
}

func List(ctx context.Context, pool *pgxpool.Pool, now time.Time) ([]Status, error) {
	rows, err := gen.New(pool).ListMaintenanceJobRuns(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]Status, len(rows))
	for i, row := range rows {
		statuses[i] = Status{Job: row.Job, PeriodSeconds: int64(row.PeriodSeconds)}
		since := row.RegisteredAt.Time
		if row.SucceededAt.Valid {
			succeeded := row.SucceededAt.Time
			statuses[i].LastSucceeded, since = &succeeded, succeeded
		}
		statuses[i].OverdueRatio = OverdueRatio(since, time.Duration(row.PeriodSeconds)*time.Second, now)
	}
	return statuses, nil
}

func OverdueRatio(since time.Time, period time.Duration, now time.Time) float64 {
	if period <= 0 {
		return 0
	}
	return max(0, float64(now.Sub(since))/float64(period))
}

func PublishGauges(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		statuses, err := List(ctx, pool, time.Now())
		if err != nil {
			return err
		}
		metrics.MaintenanceOverdueRatio.Reset()
		for _, status := range statuses {
			metrics.MaintenanceOverdueRatio.WithLabelValues(status.Job).Set(status.OverdueRatio)
		}
		return nil
	}
}
