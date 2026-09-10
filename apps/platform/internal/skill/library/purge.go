package registry

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.PurgeUnreferencedSkills(ctx, workspaceID)
	return err
}

type DeletionSweep struct {
	Purged  int64
	Waiting int64
	Kept    int64
}

func (s *Service) PurgeDeletedSkills(ctx context.Context, grace time.Duration, limit int32) (DeletionSweep, error) {
	if grace <= 0 {

		return DeletionSweep{}, errors.New("registry: deletion grace period must be positive")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeletionSweep{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return DeletionSweep{}, err
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-grace), Valid: true}
	q := gen.New(tx)
	purged, err := q.PurgeSkillsPastDeletionGrace(ctx, gen.PurgeSkillsPastDeletionGraceParams{
		Cutoff: cutoff, RowLimit: limit,
	})
	if err != nil {
		return DeletionSweep{}, err
	}

	counts, err := q.CountSkillsAwaitingDeletionGrace(ctx, cutoff)
	if err != nil {
		return DeletionSweep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletionSweep{}, err
	}
	return DeletionSweep{Purged: purged, Waiting: counts.Waiting, Kept: counts.Kept}, nil
}

type ObjectRemover interface {
	Remove(ctx context.Context, key string) error
}

type Collection struct {
	Dropped   int64
	Collected int
	Depth     int64
}

func LockPackageObject(ctx context.Context, db gen.DBTX, key string) error {
	return gen.New(db).LockPackageObjectSession(ctx, key)
}

func UnlockPackageObject(ctx context.Context, db gen.DBTX, key string) error {
	_, err := gen.New(db).UnlockPackageObjectSession(ctx, key)
	return err
}

func TrackPackageObject(ctx context.Context, db gen.DBTX, key string) error {
	return gen.New(db).RememberPackageObject(ctx, key)
}

func (s *Service) CollectOrphanObjects(ctx context.Context, store ObjectRemover, limit int32) (Collection, error) {
	if store == nil {
		return Collection{}, errors.New("registry: object collection needs an object store")
	}
	q := gen.New(s.Pool)
	dropped, err := q.DropReferencedCollectionEntries(ctx)
	if err != nil {
		return Collection{}, err
	}
	result := Collection{Dropped: dropped}

	keys, err := q.ListCollectableObjects(ctx, limit)
	if err != nil {
		return result, err
	}
	for _, key := range keys {
		collected, err := s.collectPackageObject(ctx, store, key)
		if err != nil {

			slog.Warn("orphan package object not removed; will retry", "error", err)
			continue
		}
		if collected {
			result.Collected++
		}
	}

	result.Depth, err = q.CountCollectableObjects(ctx)
	return result, err
}

func (s *Service) collectPackageObject(ctx context.Context, store ObjectRemover, key string) (bool, error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	if err := LockPackageObject(ctx, conn, key); err != nil {
		return false, err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := UnlockPackageObject(unlockCtx, conn, key); err != nil {
			slog.Error("package object collection lock could not be released; closing connection", "error", err)
			_ = conn.Hijack().Close(context.Background())
		}
	}()
	q := gen.New(conn)
	collectable, err := q.PackageObjectCollectable(ctx, key)
	if err != nil {
		return false, err
	}
	if !collectable {
		return false, q.DeleteObjectCollectionEntry(ctx, key)
	}
	if err := store.Remove(ctx, key); err != nil {
		return false, err
	}
	return true, q.DeleteObjectCollectionEntry(ctx, key)
}
