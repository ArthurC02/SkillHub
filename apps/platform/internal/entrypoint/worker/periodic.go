package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
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

const BacklogObserveInterval = 15 * time.Minute

type BacklogObserveArgs struct{}

func (BacklogObserveArgs) Kind() string { return "backlog_observe" }

type backlogOldest func(context.Context) (pgtype.Timestamptz, error)

type BacklogObserveWorker struct {
	river.WorkerDefaults[BacklogObserveArgs]
	Backlogs map[string]backlogOldest
}

func (w *BacklogObserveWorker) Work(ctx context.Context, _ *river.Job[BacklogObserveArgs]) error {
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

const enrichmentBackfillBatch = 50

type EnrichmentBackfillArgs struct{}

func (EnrichmentBackfillArgs) Kind() string { return "enrichment_backfill" }

type EnrichmentBackfillWorker struct {
	river.WorkerDefaults[EnrichmentBackfillArgs]
	Svc *ingest.Service
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
		IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
			return catalogSvc.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
				SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
				EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
				Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
				EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
				EnrichmentPromptVersion: p.EnrichmentPromptVersion,
			})
		},
		PendingEnrichments: func(ctx context.Context, limit int32) ([]ingest.PendingEnrichment, error) {
			rows, err := catalogSvc.PendingEnrichments(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]ingest.PendingEnrichment, len(rows))
			for i, row := range rows {
				out[i] = ingest.PendingEnrichment{
					SkillID: row.SkillID, WorkspaceID: row.WorkspaceID,
					Name: row.Name, PackageObjectKey: row.PackageObjectKey,
				}
			}
			return out, nil
		},
	}
}
