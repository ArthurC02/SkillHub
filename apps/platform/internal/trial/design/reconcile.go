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

// ExpiredDatasetCandidates lists uploaded datasets whose retention elapsed --
// the worklist behind `maintenance purge-datasets`.
//
// 0004 created datasets_expires_at_idx for a "retention sweep" and wrote the
// design down in the comment above it ("scans for expired rows rather than
// scheduling at creation time, so a shortened retention policy applies to
// already-stored data"). The sweep was never written. So the column existed, the
// index existed, the reasoning existed, and for the whole life of the table the
// only statement that ever deleted a dataset was account deletion -- a
// participant who kept their account kept every file they had ever uploaded,
// against a 90-day figure the upload screen had already quoted them
// (DatasetRetention, surfaced as `retention_days`) and the consent form repeats.
//
// Which rows qualify is testlab's decision and stays here; when to scan is the
// sweep's. It includes expired live rows and soft-deleted rows whose first
// object removal did not complete.
func (s *Service) ExpiredDatasetCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errPersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListDatasetsPastRetention(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

// DatasetCleanupIntentCandidates lists object keys left by uploads that did not
// atomically publish a live dataset row.
func (s *Service) DatasetCleanupIntentCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errPersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListDatasetCleanupIntents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

func (s *Service) MarkDatasetCleanupIntentPurged(ctx context.Context, tx pgx.Tx, intentID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errPersistenceNotConfigured
	}
	return gen.New(tx).MarkDatasetCleanupIntentPurged(ctx, intentID)
}

// GuardDatasetObjectRemoval prevents a stale cleanup intent from deleting an
// object between a slow upload and the transaction that publishes its row.
func (s *Service) GuardDatasetObjectRemoval(
	ctx context.Context, objectKey string, action func(retain bool, tx pgx.Tx) error,
) error {
	if s == nil || s.Pool == nil {
		return errPersistenceNotConfigured
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if err := q.LockDatasetObjectKey(ctx, objectKey); err != nil {
		return err
	}
	live, err := q.CountLiveDatasetsSharingObject(ctx, objectKey)
	if err != nil {
		return err
	}
	if err := action(live > 0, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkDatasetPurged records that an expired dataset's bytes are gone.
//
// Separate from MarkDatasetObjectLost even though both statements set the same
// column: that one means the object was missing when we looked, this one means
// we removed it because its retention ran out. Collapsing them would make a file
// whose entire purpose is telling those two apart unable to tell them apart.
//
// Takes the caller transaction so the owner write commits with whatever the
// generic sweep records alongside it (iron rule 9).
func (s *Service) MarkDatasetPurged(ctx context.Context, tx pgx.Tx, datasetID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errPersistenceNotConfigured
	}
	return gen.New(tx).MarkDatasetPurged(ctx, datasetID)
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
