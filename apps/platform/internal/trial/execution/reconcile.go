package run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errReconcilePersistenceNotConfigured = errors.New("run: reconcile persistence is not configured")

type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

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

func (s *Service) MarkRunOutputPurged(ctx context.Context, tx pgx.Tx, artifactID pgtype.UUID) error {
	if s == nil || tx == nil || s.ClearSightings == nil {
		return errReconcilePersistenceNotConfigured
	}
	if err := gen.New(tx).MarkRunOutputPurged(ctx, artifactID); err != nil {
		return err
	}
	return s.ClearSightings(ctx, tx, []pgtype.UUID{artifactID})
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
