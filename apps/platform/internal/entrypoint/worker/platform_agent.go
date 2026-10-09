package worker

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
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

const proposalExpiryInterval = 15 * time.Minute

type ProposalExpiryArgs struct{}

func (ProposalExpiryArgs) Kind() string { return "platform_agent_proposal_expiry" }

type ProposalExpiryWorker struct {
	river.WorkerDefaults[ProposalExpiryArgs]
	Svc *operations.Service
}

func (w *ProposalExpiryWorker) Work(ctx context.Context, _ *river.Job[ProposalExpiryArgs]) error {
	_, expireErr := w.Svc.ExpireProposals(ctx)
	_, abandonErr := w.Svc.AbandonStaleProposals(ctx)
	return errors.Join(expireErr, abandonErr)
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
	addWorker(set, workers, &ProposalExpiryWorker{Svc: &operations.Service{Pool: pool}})
	if agentRunsAvailable(deps) {
		tools := wiring.AgentTools(pool, deps.RestoreRate, wiring.NewExposureDocket(pool, set.Registry, set.CreationSearch))
		runs := wiring.NewAgentRuns(pool, deps.LLM, deps.Gateway, creditSvc, tools)
		addWorker(set, workers, &PlatformAgentRunWorker{Run: runs})
	}
}
