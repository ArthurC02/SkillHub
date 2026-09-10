package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := gen.New(pool)
	pruned, err := q.PruneDeletedSearchDocuments(ctx)
	if err != nil {
		slog.Error("prune deleted", "error", err)
		os.Exit(1)
	}
	n, err := q.ReindexAll(ctx)
	if err != nil {
		slog.Error("reindex", "error", err)
		os.Exit(1)
	}
	slog.Info("search projection rebuilt", "documents", n, "pruned", pruned)

	filled, err := catalog.BackfillBigram(ctx, pool, 500)
	if err != nil {
		slog.Error("bigram backfill", "error", err)
		os.Exit(1)
	}
	slog.Info("bigram column filled", "documents", filled)

	llmURL := os.Getenv("LLM_SERVICE_URL")
	if llmURL == "" {
		slog.Warn("LLM_SERVICE_URL not set; skipping enrichment backfill, documents stay pending")
		return
	}
	llmToken := os.Getenv("LLM_SERVICE_TOKEN")
	if llmToken == "" {
		slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
		os.Exit(1)
	}
	store, err := objstore.FromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
	}

	catalogSvc := &catalog.Service{Pool: pool}
	svc := &ingest.Service{
		Pool: pool, Store: store,
		LLM: &llmclient.Client{BaseURL: llmURL, Token: llmToken},
		IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
			return catalog.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
				SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
				EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
				Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
				EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
				EnrichmentPromptVersion: p.EnrichmentPromptVersion,
			})
		},
		PendingEnrichments: func(ctx context.Context, limit int32) ([]ingest.PendingEnrichment, error) {
			return pendingEnrichments(ctx, catalogSvc, limit)
		},
	}

	creditCfg, err := credit.ConfigFromEnv()
	if err != nil {
		slog.Error("credit config", "error", err)
		os.Exit(1)
	}
	svc.Credit = &credit.Service{Store: credit.NewPostgresStore(pool), Config: creditCfg}

	if keep := os.Getenv("REINDEX_REENRICH"); keep != "" {
		reset, err := q.ResetCatalogueEnrichmentBefore(ctx, keep)
		if err != nil {
			slog.Error("re-enrichment reset", "error", err)
			os.Exit(1)
		}
		slog.Info("catalogue documents queued for re-enrichment", "documents", reset, "keeping", keep)
	}
	done, failed, err := svc.ReindexPending(ctx, batchSize())
	if err != nil {
		slog.Error("enrichment backfill", "error", err)
		os.Exit(1)
	}
	slog.Info("enrichment backfill complete", "enriched", done, "still_pending", failed)
}

func pendingEnrichments(ctx context.Context, svc *catalog.Service, limit int32) ([]ingest.PendingEnrichment, error) {
	rows, err := svc.PendingEnrichments(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ingest.PendingEnrichment, len(rows))
	for i, row := range rows {
		result[i] = ingest.PendingEnrichment{
			SkillID:          row.SkillID,
			WorkspaceID:      row.WorkspaceID,
			Name:             row.Name,
			PackageObjectKey: row.PackageObjectKey,
		}
	}
	return result, nil
}

func batchSize() int32 {
	if n, err := strconv.Atoi(os.Getenv("REINDEX_BATCH")); err == nil && n > 0 {
		return int32(n)
	}
	return 200
}
