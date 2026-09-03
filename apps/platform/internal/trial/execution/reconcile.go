package run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errReconcilePersistenceNotConfigured = errors.New("run: reconcile persistence is not configured")

// ReconcileCandidate is run's read model for the generic object sweep, the same
// shape packaging declares for the same reason: the sweep is handed three
// fields, not a generated artifact row whose schema would become a
// cross-context contract.
//
// Declared here rather than reused from objreconcile because objreconcile is
// its own context (db/query-owners.yaml owns object_reconcile_sightings to it)
// and a new import across that line has to be registered in ADR-032 附錄 A and
// the depguard rules in the same commit. Three fields are cheaper than that,
// and packaging already pays the same price.
type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

// ExpiredArtifactCandidates lists run outputs whose retention elapsed — the
// worklist behind `maintenance purge-run-artifacts` (PDM-006 §6 and the consent
// document §3 both promise 30 days; until this existed nothing enforced it).
//
// Which rows qualify is run's decision and stays here; when to scan is the
// sweep's. Soft-deleted rows are not in it: DeleteArtifact already removed those
// bytes, and it did so behind a shared-key count this path does not have.
func (s *Service) ExpiredArtifactCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errReconcilePersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListRunOutputsPastRetention(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

// MarkRunOutputPurged records that a Run output's bytes are gone while the row
// stays readable — WS-004 still lists it and the owner can still delete it, and
// "this expired" is a different answer than "this never existed" (0028).
//
// run's own write and not packaging's MarkArtifactPurged, which reaches the same
// physical table: the two kinds have different owners and a cross-context write
// is refused (ADR-033). Idempotent by the statement's own predicate.
//
// Takes the caller transaction so the owner write commits with whatever the
// generic sweep records alongside it (iron rule 9).
func (s *Service) MarkRunOutputPurged(ctx context.Context, tx pgx.Tx, artifactID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errReconcilePersistenceNotConfigured
	}
	return gen.New(tx).MarkRunOutputPurged(ctx, artifactID)
}

func (s *Service) ArtifactUploadIntentCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errReconcilePersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListRunArtifactUploadIntents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

func (s *Service) MarkArtifactUploadIntentPurged(ctx context.Context, tx pgx.Tx, id pgtype.UUID) error {
	if s == nil || tx == nil {
		return errReconcilePersistenceNotConfigured
	}
	return gen.New(tx).MarkRunArtifactUploadIntentPurged(ctx, id)
}

func (s *Service) GuardArtifactUploadIntentRemoval(
	ctx context.Context, objectKey string, action func(retain bool, tx pgx.Tx) error,
) error {
	if s == nil || s.Pool == nil {
		return errReconcilePersistenceNotConfigured
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if err := q.LockRunArtifactObjectKey(ctx, objectKey); err != nil {
		return err
	}
	live, err := q.CountLiveRunArtifactsSharingObject(ctx, objectKey)
	if err != nil {
		return err
	}
	if err := action(live > 0, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
