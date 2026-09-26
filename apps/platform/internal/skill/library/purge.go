package registry

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ReferenceRead func(ctx context.Context, db gen.DBTX, ids []pgtype.UUID) ([]pgtype.UUID, error)

var errPurgeReadsNotInjected = errors.New("registry: purge reference reads not injected; refusing to purge")

type purgeCandidate struct {
	skillID    pgtype.UUID
	versionIDs []pgtype.UUID
}

type purgeHolds struct {
	skills   map[pgtype.UUID]bool
	versions map[pgtype.UUID]bool
}

func (h purgeHolds) keep(c purgeCandidate) bool {
	return h.skills[c.skillID] || slices.ContainsFunc(c.versionIDs, func(id pgtype.UUID) bool { return h.versions[id] })
}

func forkedSkills(ctx context.Context, db gen.DBTX, skillIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListForkedSkills(ctx, skillIDs)
}

func (s *Service) requirePurgeReads() error {
	if s.VersionsInRuns == nil || s.VersionsInDownloads == nil || s.VersionsInBundles == nil || s.SkillsWithTestCases == nil {
		return errPurgeReadsNotInjected
	}
	return nil
}

func (s *Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if err := s.requirePurgeReads(); err != nil {
		return err
	}
	q := gen.New(tx)
	rows, err := q.ListWorkspacePurgeCandidates(ctx, workspaceID)
	if err != nil {
		return err
	}
	candidates := make([]purgeCandidate, len(rows))
	for i, r := range rows {
		candidates[i] = purgeCandidate{skillID: r.ID, versionIDs: r.VersionIds}
	}
	purgeable, _, err := s.unreferenced(ctx, tx, candidates)
	if err != nil || len(purgeable) == 0 {
		return err
	}
	_, err = q.PurgeSkillsByID(ctx, gen.PurgeSkillsByIDParams{SkillIds: purgeable})
	return err
}

func (s *Service) purgeHolds(ctx context.Context, db gen.DBTX, candidates []purgeCandidate) (purgeHolds, error) {
	var skillIDs, versionIDs []pgtype.UUID
	for _, c := range candidates {
		skillIDs = append(skillIDs, c.skillID)
		versionIDs = append(versionIDs, c.versionIDs...)
	}
	holds := purgeHolds{skills: map[pgtype.UUID]bool{}, versions: map[pgtype.UUID]bool{}}
	for _, hold := range []struct {
		read ReferenceRead
		ids  []pgtype.UUID
		held map[pgtype.UUID]bool
	}{
		{forkedSkills, skillIDs, holds.skills},
		{s.SkillsWithTestCases, skillIDs, holds.skills},
		{s.VersionsInRuns, versionIDs, holds.versions},
		{s.VersionsInDownloads, versionIDs, holds.versions},
		{s.VersionsInBundles, versionIDs, holds.versions},
	} {
		ids, err := hold.read(ctx, db, hold.ids)
		if err != nil {
			return purgeHolds{}, err
		}
		for _, id := range ids {
			hold.held[id] = true
		}
	}
	return holds, nil
}

func (s *Service) unreferenced(ctx context.Context, db gen.DBTX, candidates []purgeCandidate) ([]pgtype.UUID, int64, error) {
	holds, err := s.purgeHolds(ctx, db, candidates)
	if err != nil {
		return nil, 0, err
	}
	var purgeable []pgtype.UUID
	var kept int64
	for _, c := range candidates {
		if holds.keep(c) {
			kept++
			continue
		}
		purgeable = append(purgeable, c.skillID)
	}
	return purgeable, kept, nil
}

func SourcesInVersions(ctx context.Context, db gen.DBTX, sourceIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListSkillSourcesInVersions(ctx, sourceIDs)
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
	if err := s.requirePurgeReads(); err != nil {
		return DeletionSweep{}, err
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
	rows, err := q.ListSkillsPastDeletionGrace(ctx, cutoff)
	if err != nil {
		return DeletionSweep{}, err
	}
	candidates := make([]purgeCandidate, len(rows))
	for i, r := range rows {
		candidates[i] = purgeCandidate{skillID: r.ID, versionIDs: r.VersionIds}
	}
	purgeable, kept, err := s.unreferenced(ctx, tx, candidates)
	if err != nil {
		return DeletionSweep{}, err
	}
	purgeable = purgeable[:min(len(purgeable), max(int(limit), 0))]
	var purged int64
	if len(purgeable) > 0 {
		purged, err = q.PurgeSkillsByID(ctx, gen.PurgeSkillsByIDParams{SkillIds: purgeable, Cutoff: cutoff})
		if err != nil {
			return DeletionSweep{}, err
		}
	}

	waiting, err := q.CountSkillsWaitingForDeletionGrace(ctx, cutoff)
	if err != nil {
		return DeletionSweep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletionSweep{}, err
	}
	return DeletionSweep{Purged: purged, Waiting: waiting, Kept: kept}, nil
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

func (s *Service) OldestCollectableObject(ctx context.Context) (pgtype.Timestamptz, error) {
	return gen.New(s.Pool).OldestCollectableObjectEnqueuedAt(ctx)
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
