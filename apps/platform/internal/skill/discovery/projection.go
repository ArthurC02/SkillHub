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
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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

// SkillRisks answers the projected risk block for a page of skills in one
// workspace, keyed by skill id and already serialised.
//
// Serialised rather than structured, unlike SourceByID next door, and the
// difference is the point: the caller does not read a single field of this. It
// forwards the block to the same `SearchResultRisk` schema a search row uses, so
// a mirror struct would only be a second place the shape can drift — and the one
// rule this facet exists to keep is that the two planes word one fact the same
// way (02:NFR-007 第 3 條). Nothing drifts if nobody re-declares it.
//
// Every requested id gets an entry. A skill with no document, and one whose
// document carries no scan, both get the same 未測量 block a search row gets:
// silence is the one answer DISC-004 forbids. The commonest case by far is a
// fork — IndexSkill writes name and summary only, because a fork shares its
// source's bytes and has nothing of its own to scan.
func (s *Service) SkillRisks(
	ctx context.Context, workspaceID pgtype.UUID, skillIDs []pgtype.UUID,
) (map[string]json.RawMessage, error) {
	unknown, err := json.Marshal(riskHint(nil))
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(skillIDs))
	for _, id := range skillIDs {
		out[pgconv.UUIDString(id)] = unknown
	}
	if len(skillIDs) == 0 {
		return out, nil
	}

	rows, err := gen.New(s.Pool).ListSkillScans(ctx, gen.ListSkillScansParams{
		WorkspaceID: workspaceID, SkillIds: skillIDs,
	})
	if err != nil {
		return nil, err
	}
	return fillScans(out, rows, func(r gen.ListSkillScansRow) (pgtype.UUID, []byte) {
		return r.SkillID, r.Scan
	})
}

// CatalogSkillRisks is SkillRisks against the public catalogue instead of one
// workspace, for the one caller that legitimately reads outside its own scope:
// a fork whose bytes are byte-identical to a catalogue ancestor shows the
// ancestor's scan (ADR-042 決策 6). The scope is baked into the SQL and there is
// no workspace argument, exactly like GetCatalogSkill — a widened scope the
// caller could name is what 鐵律 3 forbids, and a private ancestor therefore
// simply does not come back.
func (s *Service) CatalogSkillRisks(
	ctx context.Context, skillIDs []pgtype.UUID,
) (map[string]json.RawMessage, error) {
	unknown, err := json.Marshal(riskHint(nil))
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(skillIDs))
	for _, id := range skillIDs {
		out[pgconv.UUIDString(id)] = unknown
	}
	if len(skillIDs) == 0 {
		return out, nil
	}

	rows, err := gen.New(s.Pool).ListCatalogSkillScans(ctx, skillIDs)
	if err != nil {
		return nil, err
	}
	return fillScans(out, rows, func(r gen.ListCatalogSkillScansRow) (pgtype.UUID, []byte) {
		return r.SkillID, r.Scan
	})
}

// fillScans replaces the 未測量 placeholder for every row that carries a scan.
// A row with no document, and one whose document has no scan, keeps it: silence
// is the one answer DISC-004 forbids.
func fillScans[R any](
	out map[string]json.RawMessage, rows []R, split func(R) (pgtype.UUID, []byte),
) (map[string]json.RawMessage, error) {
	for _, row := range rows {
		id, scan := split(row)
		if len(scan) == 0 {
			continue
		}
		blob, err := json.Marshal(riskHint(scan))
		if err != nil {
			return nil, err
		}
		out[pgconv.UUIDString(id)] = blob
	}
	return out, nil
}
