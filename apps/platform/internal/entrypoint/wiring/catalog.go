package wiring

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func NewCatalogService(pool *pgxpool.Pool) *catalog.Service {
	return &catalog.Service{
		Pool: pool,
		ReadLiveListingFacts: func(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) (catalog.ListingFacts, bool, error) {
			row, live, err := registry.LiveListingFacts(ctx, db, skillID)
			return catalog.ListingFacts{
				Redistribution: row.Redistribution, Category: row.Category, CategorySource: row.CategorySource,
				CurationTier: row.CurationTier, CuratedVersionID: row.CuratedVersionID,
				LatestVersionID: row.LatestVersionID, VerifiedAt: row.VerifiedAt,
				LatestPackageObjectKey: row.LatestPackageObjectKey,
				LatestSourcePath:       row.LatestSourcePath,
				AgentCapability:        row.AgentCapability, AgentRuntime: row.AgentRuntime,
				AgentRuntimeImage: row.AgentRuntimeImage, AgentMeasuredAt: row.AgentMeasuredAt,
			}, live, err
		},
		ReadLiveSkills: func(ctx context.Context, db gen.DBTX) ([]catalog.IndexSkillFacts, error) {
			rows, err := registry.LiveSkills(ctx, db)
			if err != nil {
				return nil, err
			}
			facts := make([]catalog.IndexSkillFacts, len(rows))
			for i, row := range rows {
				facts[i] = catalog.IndexSkillFacts{
					ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name,
					Summary: row.Summary, Redistribution: row.Redistribution,
				}
			}
			return facts, nil
		},
		ReadLiveSkillIDs: registry.LiveSkillIDs,
	}
}
