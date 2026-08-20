package packaging

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// MarkArtifactPurged records that a Download Artifact's bytes are gone while the
// row stays readable — "this expired" is a different answer to 02:WS-002 than
// "this never existed" (0028). Idempotent by the statement's own predicate, so a
// sweep interrupted anywhere is safe to run again (iron rule 9).
//
// It exists because `artifacts` is packaging's table while the sweep that finds
// the rows (internal/objreconcile) is a generic scanner with no domain rules
// (ADR-032 §1): the scanner reports the difference, the owner applies it
// (ADR-033 clearance path 4). objreconcile reaches this through an injected
// function rather than an import — a generic subdomain importing a context would
// be the layering upside down.
//
// It takes the caller's *gen.Queries and never opens anything of its own,
// because the two callers need different things from the same write: the
// retention half writes on the pool, the existence half writes inside the
// transaction that also carries the audit event and the sighting row (iron rule
// 9). A function that began its own transaction could serve neither.
func MarkArtifactPurged(ctx context.Context, q *gen.Queries, artifactID pgtype.UUID) error {
	return q.MarkArtifactPurged(ctx, artifactID)
}
