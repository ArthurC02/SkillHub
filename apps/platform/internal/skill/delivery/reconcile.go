package packaging

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errReconcilePersistenceNotConfigured = errors.New("packaging: reconcile persistence is not configured")

// ReconcileCandidate is packaging's read model for the generic object scanner.
// It intentionally contains only what that scanner needs, not a generated
// artifact row whose schema would become a cross-context contract.
type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

// ActiveArtifactReferences answers whether any live artifact still needs an
// object. The Service owns the Pool because this read belongs to packaging.
func (s *Service) ActiveArtifactReferences(ctx context.Context, objectKey string) (int64, error) {
	return gen.New(s.Pool).CountArtifactsSharingObject(ctx, objectKey)
}

// ExpiredReconcileCandidates lists download packages whose retention elapsed.
func (s *Service) ExpiredReconcileCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errReconcilePersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListArtifactsPastRetention(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

// ClaimedReconcileCandidates lists live download packages that still claim an
// object exists and therefore need probing.
func (s *Service) ClaimedReconcileCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errReconcilePersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListArtifactsClaimingObject(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

// MarkArtifactPurged records that a Download Artifact's bytes are gone while the
// row stays readable — "this expired" is a different answer to 02:WS-002 than
// "this never existed" (0028). Idempotent by the statement's own predicate, so a
// sweep interrupted anywhere is safe to run again (iron rule 9).
//
// It exists because `artifacts` is packaging's table while the sweep that finds
// the rows (internal/foundation/storage/objreconcile) is a generic scanner with no domain rules
// (ADR-032 §1): the scanner reports the difference, the owner applies it
// (ADR-033 clearance path 4). objreconcile reaches this through an injected
// function rather than an import — a generic subdomain importing a context would
// be the layering upside down.
//
// It takes the caller transaction so the existence correction, audit event and
// sighting cleanup cannot commit apart (iron rule 9).
func (s *Service) MarkArtifactPurged(ctx context.Context, tx pgx.Tx, artifactID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errReconcilePersistenceNotConfigured
	}
	return gen.New(tx).MarkArtifactPurged(ctx, artifactID)
}
