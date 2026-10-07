package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

const PartitionCreateInterval = 24 * time.Hour

type PartitionCreateArgs struct{}

func (PartitionCreateArgs) Kind() string { return "partition_create" }

type PartitionCreateWorker struct {
	river.WorkerDefaults[PartitionCreateArgs]
	Pool *pgxpool.Pool
}

func partitionedTables() []string {
	return []string{analytics.PartitionedTable, trace.PartitionedTable}
}

type CapacitySampleArgs struct{}

func (CapacitySampleArgs) Kind() string { return "capacity_sample" }

type CapacitySampleWorker struct {
	river.WorkerDefaults[CapacitySampleArgs]
	Store capacity.Store
}

func (w *CapacitySampleWorker) Work(ctx context.Context, _ *river.Job[CapacitySampleArgs]) error {
	return w.Store.RecordToday(ctx, time.Now())
}

func (w *PartitionCreateWorker) Work(ctx context.Context, _ *river.Job[PartitionCreateArgs]) error {
	var failures []error
	for _, table := range partitionedTables() {

		report, err := partition.CreateUpcoming(ctx, w.Pool, table, time.Now())
		if len(report.Created) > 0 {
			slog.Info("partitions created", "table", table, "partitions", report.Created)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

type BacklogObserveArgs struct{}

func (BacklogObserveArgs) Kind() string { return "backlog_observe" }

type backlogOldest func(context.Context) (pgtype.Timestamptz, error)

type BacklogObserveWorker struct {
	river.WorkerDefaults[BacklogObserveArgs]
	Backlogs map[string]backlogOldest
}

func (w *BacklogObserveWorker) Work(ctx context.Context, _ *river.Job[BacklogObserveArgs]) error {
	return w.Observe(ctx)
}

func (w *BacklogObserveWorker) Observe(ctx context.Context) error {
	now := time.Now()
	var failures []error
	for name, oldest := range w.Backlogs {
		at, err := oldest(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("observe the %s backlog: %w", name, err))
			continue
		}
		metrics.BacklogOldestSeconds.WithLabelValues(name).Set(backlogAge(at, now))
	}
	return errors.Join(failures...)
}

func backlogAge(oldest pgtype.Timestamptz, now time.Time) float64 {
	if !oldest.Valid {
		return 0
	}
	return max(0, now.Sub(oldest.Time).Seconds())
}

const EnrichmentBackfillInterval = time.Hour

const (
	enrichmentBackfillBatch   = 50
	enrichmentBackfillTimeout = 15 * time.Minute
)

type EnrichmentBackfillArgs struct{}

func (EnrichmentBackfillArgs) Kind() string { return "enrichment_backfill" }

type EnrichmentBackfillWorker struct {
	river.WorkerDefaults[EnrichmentBackfillArgs]
	Svc *ingest.Service
}

func (*EnrichmentBackfillWorker) Timeout(*river.Job[EnrichmentBackfillArgs]) time.Duration {
	return enrichmentBackfillTimeout
}

func (w *EnrichmentBackfillWorker) Work(ctx context.Context, _ *river.Job[EnrichmentBackfillArgs]) error {
	if w.Svc == nil {
		return nil
	}
	done, failed, err := w.Svc.ReindexPending(ctx, enrichmentBackfillBatch)
	if err != nil {
		return fmt.Errorf("enrichment backfill: %w", err)
	}
	if done > 0 || failed > 0 {
		slog.Info("enrichment backfill pass", "enriched", done, "still_pending", failed)
	}
	return nil
}

func newBackfillService(pool *pgxpool.Pool, deps Deps) *ingest.Service {
	if deps.LLM == nil || deps.Store == nil {
		return nil
	}
	catalogSvc := wiring.NewCatalogService(pool)
	return &ingest.Service{
		Pool: pool, Store: deps.Store, LLM: ingest.ModelOrNone(deps.LLM),
		Budgets:            wiring.NewModelBudgets(pool),
		IndexSkill:         wiring.EnrichedIndexer(catalogSvc),
		PendingEnrichments: wiring.PendingEnrichments(catalogSvc),
	}
}

const GaugeRefreshInterval = time.Minute

func RefreshGauges(ctx context.Context, publishers []func(context.Context) error) {
	ticker := time.NewTicker(GaugeRefreshInterval)
	defer ticker.Stop()
	for {
		round, cancel := context.WithTimeout(ctx, GaugeRefreshInterval/2)
		for _, publish := range publishers {
			if err := publish(round); err != nil && ctx.Err() == nil {
				slog.Warn("a gauge kept its last value; refreshing it failed", "error", err)
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
