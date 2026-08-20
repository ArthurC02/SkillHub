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
// Every function takes the caller's *gen.Queries and never opens a transaction
// of its own. That is the point of the exercise: INGEST-009 requires the
// document to be written in the same transaction as the version row, and
// registry needs the takedown flag and the document's removal to commit
// together or not at all. A function that began its own transaction would turn
// one guarantee into two that can each fail alone.

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// IndexSkill writes the name-and-summary document, which is all a fork has:
// it shares its source's package bytes, so there is nothing new to enrich, and
// the enrichment columns stay at their defaults until a version of its own
// arrives.
func IndexSkill(ctx context.Context, q *gen.Queries, arg gen.UpsertSearchDocumentParams) error {
	return q.UpsertSearchDocument(ctx, arg)
}

// IndexSkillEnriched writes the full document including the ADR-013 index-time
// enhancement fields. The caller computes them — the enrichment pipeline lives
// with the package reader in ingest — so this adds no rules of its own and the
// argument stays the generated parameter struct rather than a second copy of
// the same thirteen fields.
func IndexSkillEnriched(ctx context.Context, q *gen.Queries, arg gen.UpsertSearchDocumentEnrichedParams) error {
	return q.UpsertSearchDocumentEnriched(ctx, arg)
}

// RemoveSkillFromIndex drops a skill's document. Called for both soft delete
// and takedown: in either case the content must stop being discoverable now,
// while the version snapshots it owns stay frozen (iron rule 4).
func RemoveSkillFromIndex(ctx context.Context, q *gen.Queries, skillID pgtype.UUID) error {
	return q.DeleteSearchDocument(ctx, skillID)
}
