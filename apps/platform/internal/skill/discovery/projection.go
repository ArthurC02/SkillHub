package catalog

// The write half of the search projection. `search_documents` is catalog's
// table — the field semantics (enriched summary, task examples, tags, the
// embedding computed from them) are decided by the retrieval pipeline that
// reads them — but the rows are produced by whoever changed the source data:
// registry when a skill is forked, deleted or taken down, ingest when a version
// lands or the enrichment backfill catches up.
//
// Those two reach these functions through an injected field rather than an
// import (ADR-034). `catalog -> registry` already exists since DDD-020, so
// `registry -> catalog` would be a compile-time cycle; `ingest -> catalog` would
// not, but closing one drift with two different shapes would read as if the
// difference meant something. The composition roots — apiserver.NewApp and
// cmd/reindex — are the only places the three packages meet.
//
// Every write takes the caller's transaction and never opens one of its own.
// That is the point of the exercise: INGEST-009 requires the
// document to be written in the same transaction as the version row, and
// registry needs the takedown flag and the document's removal to commit
// together or not at all. A function that began its own transaction would turn
// one guarantee into two that can each fail alone.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PendingEnrichment is the catalog-owned view consumed by the enrichment backfill.
type PendingEnrichment struct {
	SkillID          pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
}

// PendingEnrichments keeps the generated search projection row inside catalog.
func (s *Service) PendingEnrichments(ctx context.Context, limit int32) ([]PendingEnrichment, error) {
	rows, err := gen.New(s.Pool).ListPendingEnrichment(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]PendingEnrichment, len(rows))
	for i, row := range rows {
		result[i] = PendingEnrichment{
			SkillID:          row.SkillID,
			WorkspaceID:      row.WorkspaceID,
			Name:             row.Name,
			PackageObjectKey: row.PackageObjectKey,
		}
	}
	return result, nil
}

// SkillProjection is catalog's transaction contract for a basic search row.
type SkillProjection struct {
	SkillID     pgtype.UUID
	WorkspaceID pgtype.UUID
	Name        string
	Summary     string
}

// EnrichedSkillProjection is catalog's transaction contract for a complete search row.
type EnrichedSkillProjection struct {
	SkillID                 pgtype.UUID
	WorkspaceID             pgtype.UUID
	Name                    string
	Summary                 string
	EnrichedSummary         string
	TaskExamples            string
	Tags                    []byte
	Limitations             string
	Scan                    []byte
	Embedding               *pgvector.Vector
	EnrichmentStatus        string
	EnrichmentModel         *string
	EnrichmentPromptVersion *string
}

// IndexSkill writes the name-and-summary document, which is all a fork has:
// it shares its source's package bytes, so there is nothing new to enrich, and
// the enrichment columns stay at their defaults until a version of its own
// arrives.
func IndexSkill(ctx context.Context, tx pgx.Tx, projection SkillProjection) error {
	return gen.New(tx).UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
		SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
		Name: projection.Name, Summary: projection.Summary,
	})
}

// IndexSkillEnriched writes the full document including the ADR-013 index-time
// enhancement fields. The caller computes them — the enrichment pipeline lives
// with the package reader in ingest — so this adds no rules of its own and the
// argument is catalog-owned so generated database types do not cross contexts.
func IndexSkillEnriched(ctx context.Context, tx pgx.Tx, projection EnrichedSkillProjection) error {
	return gen.New(tx).UpsertSearchDocumentEnriched(ctx, gen.UpsertSearchDocumentEnrichedParams{
		SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
		Name: projection.Name, Summary: projection.Summary,
		EnrichedSummary: projection.EnrichedSummary, TaskExamples: projection.TaskExamples,
		Tags: projection.Tags, Limitations: projection.Limitations, Scan: projection.Scan,
		Embedding: projection.Embedding, EnrichmentStatus: projection.EnrichmentStatus,
		EnrichmentModel:         projection.EnrichmentModel,
		EnrichmentPromptVersion: projection.EnrichmentPromptVersion,
	})
}

// RemoveSkillFromIndex drops a skill's document. Called for both soft delete
// and takedown: in either case the content must stop being discoverable now,
// while the version snapshots it owns stay frozen (iron rule 4).
func RemoveSkillFromIndex(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID) error {
	return gen.New(tx).DeleteSearchDocument(ctx, skillID)
}
