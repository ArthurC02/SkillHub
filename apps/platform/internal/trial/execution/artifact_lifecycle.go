package run

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const artifactCleanupTimeout = 5 * time.Second

func (s *Service) DeleteArtifact(
	ctx context.Context, ws identity.Workspace, runID, artifactID pgtype.UUID,
) error {
	if s.ActiveArtifactReferences == nil {
		return errors.New("run: artifact reference counter not injected; refusing delete")
	}
	lookup, err := gen.New(s.Pool).GetRunArtifactForDelete(ctx, gen.GetRunArtifactForDeleteParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	lockKey := "artifact-object:" + lookup.ObjectKey
	locked := false
	defer func() {
		if !locked {
			conn.Release()
			return
		}
		releaseArtifactObjectLock(conn, lockKey)
	}()
	if err := gen.New(conn).LockRunArtifactObjectSession(ctx, lockKey); err != nil {
		return err
	}
	locked = true
	row, deleted, err := softDeleteRecorded(ctx, conn, ws, runID, artifactID)
	if err != nil || !deleted {
		return err
	}
	if row.PurgedAt.Valid || s.Store == nil {
		return nil
	}
	purgeUnreferencedObject(ctx, s, conn, row)
	return nil
}

func releaseArtifactObjectLock(conn *pgxpool.Conn, lockKey string) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), artifactCleanupTimeout)
	defer cancel()
	if _, err := gen.New(conn).UnlockRunArtifactObjectSession(unlockCtx, lockKey); err != nil {
		slog.Error("run artifact object lock could not be released; closing connection", "error", err)
		_ = conn.Hijack().Close(context.Background())
		return
	}
	conn.Release()
}

func softDeleteRecorded(
	ctx context.Context, conn *pgxpool.Conn, ws identity.Workspace, runID, artifactID pgtype.UUID,
) (gen.SoftDeleteRunArtifactRow, bool, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return gen.SoftDeleteRunArtifactRow{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	row, err := q.SoftDeleteRunArtifact(ctx, gen.SoftDeleteRunArtifactParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.SoftDeleteRunArtifactRow{}, false, nil
	}
	if err != nil {
		return gen.SoftDeleteRunArtifactRow{}, false, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDelete, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ID,
		Metadata:   map[string]any{"run_id": pgconv.UUIDString(runID)},
	}); err != nil {
		return gen.SoftDeleteRunArtifactRow{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.SoftDeleteRunArtifactRow{}, false, err
	}
	return row, true, nil
}

func purgeUnreferencedObject(
	ctx context.Context, s *Service, conn *pgxpool.Conn, row gen.SoftDeleteRunArtifactRow,
) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artifactCleanupTimeout)
	defer cancel()
	shared, err := s.ActiveArtifactReferences(cleanupCtx, conn, row.ObjectKey)
	if err == nil && shared == 0 {
		err = s.Store.Remove(cleanupCtx, row.ObjectKey)
	}
	if err == nil {
		err = pgx.BeginFunc(cleanupCtx, conn, func(tx pgx.Tx) error {
			return s.MarkRunOutputPurged(cleanupCtx, tx, row.ID)
		})
	}
	if err != nil {
		slog.Warn("run artifact object not removed; cleanup will retry", "object_key", row.ObjectKey, "error", err)
	}
}
