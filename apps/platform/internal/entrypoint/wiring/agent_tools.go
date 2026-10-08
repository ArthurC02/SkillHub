package wiring

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

type CapacityFacts struct {
	DatabaseBytes         int64            `json:"database_bytes"`
	RestoreBytesPerSecond int64            `json:"restore_bytes_per_second"`
	RestoreRateMeasured   bool             `json:"restore_rate_measured"`
	RestoreBudgetBytes    int64            `json:"restore_budget_bytes"`
	DaysUntilBudget       *float64         `json:"days_until_budget"`
	GrowthBytesPerDay     float64          `json:"growth_bytes_per_day"`
	TableBytes            map[string]int64 `json:"table_bytes"`
}

type JobFacts struct {
	PeriodSeconds int64      `json:"period_seconds"`
	LastSucceeded *time.Time `json:"last_succeeded_at"`
	OverdueRatio  float64    `json:"overdue_ratio"`
	Action        string     `json:"action,omitempty"`
}

type MaintenanceFacts struct {
	Capacity        CapacityFacts       `json:"capacity"`
	MaintenanceJobs map[string]JobFacts `json:"maintenance_jobs"`
}

func NewMaintenanceFacts(report capacity.Report, jobs []jobruns.Status) MaintenanceFacts {
	facts := MaintenanceFacts{
		Capacity: CapacityFacts{
			DatabaseBytes: report.DatabaseBytes, RestoreBytesPerSecond: report.RestoreBytesPerSecond,
			RestoreRateMeasured: report.RestoreRateMeasured, RestoreBudgetBytes: report.RestoreBudgetBytes,
			DaysUntilBudget: report.DaysUntilBudget, GrowthBytesPerDay: report.GrowthBytesPerDay,
			TableBytes: make(map[string]int64, len(report.Tables)),
		},
		MaintenanceJobs: make(map[string]JobFacts, len(jobs)),
	}
	for _, table := range report.Tables {
		facts.Capacity.TableBytes[table.Table] = table.Bytes
	}
	for _, job := range jobs {
		facts.MaintenanceJobs[job.Job] = JobFacts{
			PeriodSeconds: job.PeriodSeconds, LastSucceeded: job.LastSucceeded, OverdueRatio: job.OverdueRatio,
			Action: proposableAction(job),
		}
	}
	return facts
}

const rerunOverdueRatio = 2

func proposableAction(job jobruns.Status) string {
	if job.OverdueRatio <= rerunOverdueRatio || !slices.Contains(operations.ProposableMaintenanceJobs, job.Job) {
		return ""
	}
	return operations.MaintenanceJobAction(job.Job)
}

func MaintenanceReportTool(pool *pgxpool.Pool, rate capacity.RestoreRate, now func() time.Time) operations.Tool {
	return operations.Tool{
		Name: operations.ToolMaintenanceReport,
		Description: "The platform's maintenance facts: database and per-table size in bytes, daily growth, " +
			"the restore budget and days until it is crossed, and each scheduled maintenance job's period, " +
			"last success and overdue ratio (time since last success over its period), and the action that " +
			"runs the job now when one can be proposed. Takes no arguments.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			at := now()
			report, err := capacity.Store{Pool: pool, Rate: rate}.Report(ctx, at)
			if err != nil {
				return nil, err
			}
			jobs, err := jobruns.List(ctx, pool, at)
			if err != nil {
				return nil, err
			}
			return NewMaintenanceFacts(report, jobs), nil
		},
	}
}
