// Command reindex rebuilds the search projection from skills (INGEST-009).
// The projection is never a source of truth (ADR-010), so this is safe to run
// at any time and is the recovery path if the projection drifts or is lost.
//
// Two phases:
//
//  1. SQL rebuild — every live skill gets a document, stale ones are pruned.
//     Needs only DATABASE_URL.
//  2. Enrichment backfill — documents left pending by a failed or skipped
//     index-time enrichment are recomputed from their stored package
//     (ADR-013 §1). Needs LLM_SERVICE_URL and object storage; skipped with a
//     warning when LLM_SERVICE_URL is unset, so phase 1 still works alone.
//
// The backfill is manual on purpose: iron rule 6 puts retry decisions in Go,
// and for now that decision is an operator running this command. Re-running is
// harmless — enriched documents leave the worklist, failures stay pending.
// REINDEX_BATCH caps one run (default 200).
//
// This process's composition root is main() itself, and it wires almost nothing:
// phase 1 is two generated queries with no service behind them, and phase 2
// builds the one ingest.Service the backfill needs, after the phase that does not
// need it has already succeeded. That ordering is the reason it is not wired up
// front (ADR-032 §5: apiserver.NewApp is the API's root, not the platform's).
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
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

	// The backfill rewrites catalog's documents, so it needs catalog's write
	// injected exactly as the API's import path does (ADR-034); ReindexPending
	// refuses to spend an enrichment call without it.
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
