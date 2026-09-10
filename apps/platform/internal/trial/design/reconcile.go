package testlab

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

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

func (s *Service) MarkDatasetPurged(ctx context.Context, tx pgx.Tx, datasetID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errPersistenceNotConfigured
	}
	return gen.New(tx).MarkDatasetPurged(ctx, datasetID)
}

func (s *Service) MarkDatasetObjectLost(ctx context.Context, tx pgx.Tx, datasetID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errPersistenceNotConfigured
	}
	return gen.New(tx).MarkDatasetObjectLost(ctx, datasetID)
}
