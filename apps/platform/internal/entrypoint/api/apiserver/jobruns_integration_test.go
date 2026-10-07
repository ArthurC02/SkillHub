package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
)

func TestMaintenanceJobsAreRegisteredForgottenAndTimedFromTheirLastSuccess(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM maintenance_job_runs`) })

	if _, err := pool.Exec(ctx, `INSERT INTO maintenance_job_runs (job, period_seconds, registered_at)
		VALUES ('retired-job', 86400, now() - interval '30 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO maintenance_job_runs (job, period_seconds, registered_at)
		VALUES ('weekly-job', 60, now() - interval '3 days')`); err != nil {
		t.Fatal(err)
	}
	if err := jobruns.Register(ctx, pool, []jobruns.Job{
		{Name: "daily-job", Period: 24 * time.Hour},
		{Name: "weekly-job", Period: 7 * 24 * time.Hour},
	}); err != nil {
		t.Fatal(err)
	}
	if err := jobruns.RecordSuccess(ctx, pool, "daily-job"); err != nil {
		t.Fatal(err)
	}
	if err := jobruns.RecordSuccess(ctx, pool, "retired-job"); err == nil {
		t.Error("recording a success for an unregistered job returned no error")
	}

	statuses, err := jobruns.List(ctx, pool, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	byJob := map[string]jobruns.Status{}
	for _, status := range statuses {
		byJob[status.Job] = status
	}
	if len(byJob) != 2 {
		t.Fatalf("jobs = %v, want daily-job and weekly-job only; the retired job should be forgotten", byJob)
	}
	daily := byJob["daily-job"]
	if daily.LastSucceeded == nil || daily.OverdueRatio > 0.01 || daily.PeriodSeconds != 86400 {
		t.Errorf("daily-job = %+v, want a fresh success, ratio near 0, period 86400", daily)
	}
	weekly := byJob["weekly-job"]
	if weekly.LastSucceeded != nil || weekly.PeriodSeconds != 604800 {
		t.Errorf("weekly-job = %+v, want no success yet and its period re-registered to a week", weekly)
	}
	if weekly.OverdueRatio < 0.42 || weekly.OverdueRatio > 0.43 {
		t.Errorf("weekly-job ratio = %v, want about 3/7 counted from when it was first registered", weekly.OverdueRatio)
	}
}
