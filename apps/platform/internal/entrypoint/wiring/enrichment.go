package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func EnrichedIndexer(catalogSvc *catalog.Service) func(context.Context, pgx.Tx, ingest.SkillProjection) error {
	return func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
		return catalogSvc.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
			SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
			EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
			Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
			EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
			EnrichmentPromptVersion: p.EnrichmentPromptVersion, EnrichmentFindings: p.EnrichmentFindings,
		})
	}
}

func EnrichmentWorklist(backfill *catalog.EnrichmentBackfill) ingest.EnrichmentWorklist {
	return ingest.EnrichmentWorklist{
		Pending: func(ctx context.Context, limit int32) ([]ingest.PendingEnrichment, error) {
			rows, err := backfill.Pending(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]ingest.PendingEnrichment, len(rows))
			for i, row := range rows {
				out[i] = ingest.PendingEnrichment(row)
			}
			return out, nil
		},
		EmbedMissing: func(ctx context.Context, limit int32, embed func(context.Context, ingest.MissingEmbedding) *pgvector.Vector) (int, int, error) {
			return backfill.EmbedMissing(ctx, limit, func(ctx context.Context, doc catalog.MissingEmbedding) *pgvector.Vector {
				return embed(ctx, ingest.MissingEmbedding(doc))
			})
		},
	}
}
