package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
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
		LLM:                ingest.ModelOrNone(wiring.LLMClient(llmURL, llmToken)),
		Budgets:            wiring.NewModelBudgets(pool),
		IndexSkill:         wiring.EnrichedIndexer(catalogSvc),
		PendingEnrichments: wiring.PendingEnrichments(catalogSvc),
	}

	if svc.Credit, err = wiring.NewCreditService(pool); err != nil {
		slog.Error("credit config", "error", err)
		return 1
	}

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

func batchSize() int32 {
	return envx.PositiveInt32("REINDEX_BATCH", defaultEnrichmentBatch)
}
