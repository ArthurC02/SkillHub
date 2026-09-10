package packaging

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errReconcilePersistenceNotConfigured = errors.New("packaging: reconcile persistence is not configured")

func downloadObjectLockKey(objectKey string) string {
	return "artifact-object:" + strings.TrimPrefix(objectKey, "/")
}

type ReconcileCandidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

func (*Service) ActiveArtifactReferences(ctx context.Context, db gen.DBTX, objectKey string) (int64, error) {
	return gen.New(db).CountArtifactsSharingObject(ctx, objectKey)
}

func (s *Service) GuardArtifactRemoval(
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
	lockKey := downloadObjectLockKey(objectKey)
	if err := gen.New(tx).LockDownloadObjectKey(ctx, lockKey); err != nil {
		return err
	}
	live, err := gen.New(tx).CountArtifactsSharingObject(ctx, objectKey)
	if err != nil {
		return err
	}
	if err := action(live > 0, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

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

func (s *Service) DownloadCleanupIntentCandidates(ctx context.Context, limit int32) ([]ReconcileCandidate, error) {
	if s == nil || s.Pool == nil {
		return nil, errReconcilePersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListDownloadCleanupIntents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileCandidate, len(rows))
	for i, row := range rows {
		out[i] = ReconcileCandidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
	}
	return out, nil
}

func (s *Service) MarkDownloadCleanupIntentPurged(ctx context.Context, tx pgx.Tx, id pgtype.UUID) error {
	if s == nil || tx == nil {
		return errReconcilePersistenceNotConfigured
	}
	return gen.New(tx).MarkDownloadCleanupIntentPurged(ctx, id)
}

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

func (s *Service) MarkArtifactPurged(ctx context.Context, tx pgx.Tx, artifactID pgtype.UUID) error {
	if s == nil || tx == nil {
		return errReconcilePersistenceNotConfigured
	}
	return gen.New(tx).MarkArtifactPurged(ctx, artifactID)
}
