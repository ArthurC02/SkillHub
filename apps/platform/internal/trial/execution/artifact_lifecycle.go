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

type artifactLifecycle struct {
	pool             *pgxpool.Pool
	store            ObjectStore
	activeReferences func(context.Context, gen.DBTX, string) (int64, error)
	markPurged       func(context.Context, pgx.Tx, pgtype.UUID) error
}

func (l artifactLifecycle) Delete(
	ctx context.Context, ws identity.Workspace, runID, artifactID pgtype.UUID,
) error {
	if l.activeReferences == nil {
		return errors.New("run: artifact reference counter not injected; refusing delete")
	}
	lookup, err := gen.New(l.pool).GetRunArtifactForDelete(ctx, gen.GetRunArtifactForDeleteParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	conn, err := l.pool.Acquire(ctx)
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
		unlockCtx, cancel := context.WithTimeout(context.Background(), artifactCleanupTimeout)
		defer cancel()
		if _, err := gen.New(conn).UnlockRunArtifactObjectSession(unlockCtx, lockKey); err != nil {
			slog.Error("run artifact object lock could not be released; closing connection", "error", err)
			_ = conn.Hijack().Close(context.Background())
			return
		}
		conn.Release()
	}()
	if err := gen.New(conn).LockRunArtifactObjectSession(ctx, lockKey); err != nil {
		return err
	}
	locked = true
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	row, err := q.SoftDeleteRunArtifact(ctx, gen.SoftDeleteRunArtifactParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDelete, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ID,
		Metadata:   map[string]any{"run_id": pgconv.UUIDString(runID)},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if row.PurgedAt.Valid || l.store == nil {
		return nil
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artifactCleanupTimeout)
	defer cancel()
	shared, err := l.activeReferences(cleanupCtx, conn, row.ObjectKey)
	if err == nil && shared == 0 {
		err = l.store.Remove(cleanupCtx, row.ObjectKey)
	}
	if err == nil {
		err = pgx.BeginFunc(cleanupCtx, conn, func(tx pgx.Tx) error {
			return l.markPurged(cleanupCtx, tx, row.ID)
		})
	}
	if err != nil {
		slog.Warn("run artifact object not removed; cleanup will retry", "object_key", row.ObjectKey, "error", err)
	}
	return nil
}
