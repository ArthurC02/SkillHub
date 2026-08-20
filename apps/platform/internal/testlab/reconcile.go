package testlab

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// MarkDatasetObjectLost stops a dataset row claiming a file that is not in
// storage any more (04 丙-9). It marks the row deleted rather than adding a
// column: every read path that cares already filters on deleted_at, so
// correcting the optimistic upper bound changes nothing on any read path.
//
// `datasets` is testlab's table; the sweep that finds the rows
// (internal/objreconcile) is a generic scanner with no domain rules (ADR-032
// §1), so it reports the difference and the owner applies it (ADR-033 clearance
// path 4). The scanner gets here through an injected function, not an import —
// a generic subdomain importing a context would be the layering upside down.
//
// It takes the caller's *gen.Queries and never opens a transaction: the sweep
// marks the row, writes the audit event and clears the sighting in one commit
// (iron rule 9), and a function that began its own would split that guarantee
// into two that can fail apart.
func MarkDatasetObjectLost(ctx context.Context, q *gen.Queries, datasetID pgtype.UUID) error {
	return q.MarkDatasetObjectLost(ctx, datasetID)
}
