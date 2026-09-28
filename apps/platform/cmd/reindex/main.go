package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func main() {
	if code := runReindex(); code != 0 {
		os.Exit(code)
	}
}

func runReindex() int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		return 1
	}
	defer pool.Close()

	catalogSvc := wiring.NewCatalogService(pool)
	n, pruned, err := catalogSvc.RebuildIndex(ctx)
	if err != nil {
		slog.Error("reindex", "error", err)
		return 1
	}
	slog.Info("search projection rebuilt", "documents", n, "pruned", pruned)

	filled, err := catalog.BackfillBigram(ctx, pool, bigramBackfillBatch)
	if err != nil {
		slog.Error("bigram backfill", "error", err)
		return 1
	}
	slog.Info("bigram column filled", "documents", filled)

	llmURL := os.Getenv("LLM_SERVICE_URL")
	if llmURL == "" {
		slog.Warn("LLM_SERVICE_URL not set; skipping enrichment backfill, documents stay pending")
		return 0
	}
	llmToken := os.Getenv("LLM_SERVICE_TOKEN")
	if llmToken == "" {
		slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
		return 1
	}
	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		return 1
	}

	svc := &ingest.Service{
		Pool: pool, Store: store,
		LLM: ingest.ModelOrNone(wiring.LLMClient(llmURL, llmToken)),
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
			return pendingEnrichments(ctx, catalogSvc, limit)
		},
	}

	creditCfg, err := wiring.CreditConfigFromEnv()
	if err != nil {
		slog.Error("credit config", "error", err)
		return 1
	}
	svc.Credit = &credit.Service{Store: credit.NewPostgresStore(pool), Config: creditCfg}

	if keep := os.Getenv("REINDEX_REENRICH"); keep != "" && requeueCatalogueForReenrichment(ctx, pool, keep) != nil {
		return 1
	}
	done, failed, err := svc.ReindexPending(ctx, batchSize())
	if err != nil {
		slog.Error("enrichment backfill", "error", err)
		return 1
	}
	slog.Info("enrichment backfill complete", "enriched", done, "still_pending", failed)
	return 0
}

const (
	bigramBackfillBatch    = 500
	defaultEnrichmentBatch = 200
)

func requeueCatalogueForReenrichment(ctx context.Context, pool *pgxpool.Pool, keep string) error {
	catalogs, err := (&identity.Service{Pool: pool}).CatalogWorkspaceIDs(ctx, pool)
	if err != nil {
		slog.Error("catalog workspaces", "error", err)
		return err
	}
	reset, err := catalog.RequeueCatalogueEnrichment(ctx, pool, catalogs, keep)
	if err != nil {
		slog.Error("re-enrichment reset", "error", err)
		return err
	}
	slog.Info("catalogue documents queued for re-enrichment", "documents", reset, "keeping", keep)
	return nil
}

func pendingEnrichments(ctx context.Context, svc *catalog.Service, limit int32) ([]ingest.PendingEnrichment, error) {
	rows, err := svc.PendingEnrichments(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ingest.PendingEnrichment, len(rows))
	for i, row := range rows {
		result[i] = ingest.PendingEnrichment{
			VersionID:        row.VersionID,
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
	return defaultEnrichmentBatch
}
