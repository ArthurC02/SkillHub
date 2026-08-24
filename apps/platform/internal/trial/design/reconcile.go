package testlab

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// ReconcileCandidate is testlab's narrow read model for the generic object
// scanner. Generated dataset rows stay inside this context.
type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

// ClaimedReconcileCandidates lists live datasets that still claim an object
// exists and therefore need probing.
func (s *Service) ClaimedReconcileCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errPersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListDatasetsClaimingObject(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

// MarkDatasetObjectLost stops a dataset row claiming a file that is not in
// storage any more (04 丙-9). It marks the row deleted rather than adding a
// column: every read path that cares already filters on deleted_at, so
// correcting the optimistic upper bound changes nothing on any read path.
//
// `datasets` is testlab's table; the sweep that finds the rows
// (internal/foundation/storage/objreconcile) is a generic scanner with no domain rules (ADR-032
// §1), so it reports the difference and the owner applies it (ADR-033 clearance
// path 4). The scanner gets here through an injected function, not an import —
// a generic subdomain importing a context would be the layering upside down.
//
// It takes the caller transaction and never opens one: the sweep
// marks the row, writes the audit event and clears the sighting in one commit
// (iron rule 9), and a function that began its own would split that guarantee
// into two that can fail apart.
func (s *Service) MarkDatasetObjectLost(ctx context.Context, tx pgx.Tx, datasetID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errPersistenceNotConfigured
	}
	return gen.New(tx).MarkDatasetObjectLost(ctx, datasetID)
}
