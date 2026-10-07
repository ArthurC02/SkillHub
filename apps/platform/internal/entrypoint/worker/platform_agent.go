package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
)

const (
	platformAgentHourUTC = 2
	platformAgentPeriod  = 24 * time.Hour
)

type PlatformAgentRunArgs struct {
	Agent string `json:"agent"`
}

func (PlatformAgentRunArgs) Kind() string { return "platform_agent_run" }

func (PlatformAgentRunArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: wiring.QueueModel, MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: platformAgentPeriod},
	}
}

type PlatformAgentRunWorker struct {
	river.WorkerDefaults[PlatformAgentRunArgs]
	Run func(ctx context.Context, agent string) error
}

func (w *PlatformAgentRunWorker) Work(ctx context.Context, job *river.Job[PlatformAgentRunArgs]) error {
	return w.Run(ctx, job.Args.Agent)
}

func (w *PlatformAgentRunWorker) Timeout(*river.Job[PlatformAgentRunArgs]) time.Duration {
	return wiring.AgentLimits.Deadline + time.Minute
}

type dailyAt struct{ hour int }

func (d dailyAt) Next(current time.Time) time.Time {
	current = current.UTC()
	next := time.Date(current.Year(), current.Month(), current.Day(), d.hour, 0, 0, 0, time.UTC)
	if !next.After(current) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func agentRunsAvailable(deps Deps) bool {
	return deps.LLM != nil && deps.Gateway != nil
}

func addCreditConsumers(set *Set, workers *river.Workers, pool *pgxpool.Pool, deps Deps, creditSvc *credit.Service) {
	addWorker(set, workers, &CreditRecomputeWorker{Svc: creditSvc})
	if agentRunsAvailable(deps) {
		runs := wiring.NewAgentRuns(pool, deps.LLM, deps.Gateway, creditSvc, deps.RestoreRate)
		addWorker(set, workers, &PlatformAgentRunWorker{Run: runs})
	}
}
