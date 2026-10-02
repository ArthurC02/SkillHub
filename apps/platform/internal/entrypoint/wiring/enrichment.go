package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"

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
			EnrichmentPromptVersion: p.EnrichmentPromptVersion,
		})
	}
}

func PendingEnrichments(catalogSvc *catalog.Service) func(context.Context, int32) ([]ingest.PendingEnrichment, error) {
	return func(ctx context.Context, limit int32) ([]ingest.PendingEnrichment, error) {
		rows, err := catalogSvc.PendingEnrichments(ctx, limit)
		if err != nil {
			return nil, err
		}
		out := make([]ingest.PendingEnrichment, len(rows))
		for i, row := range rows {
			out[i] = ingest.PendingEnrichment{
				VersionID: row.VersionID, SkillID: row.SkillID, WorkspaceID: row.WorkspaceID,
				Name: row.Name, PackageObjectKey: row.PackageObjectKey, SourcePath: row.SourcePath,
			}
		}
		return out, nil
	}
}
