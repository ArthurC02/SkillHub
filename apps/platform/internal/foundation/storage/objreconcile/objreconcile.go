package objreconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	Interval = time.Hour

	batch = 500

	actAfterRounds = 2

	kindDataset  = "dataset"
	kindArtifact = "artifact"
)

type ObjectStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Remove(ctx context.Context, key string) error
}

type MarkFunc func(ctx context.Context, tx pgx.Tx, id pgtype.UUID) error

type Candidate struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	ObjectKey   string
}

type ListFunc func(ctx context.Context, limit int32) ([]Candidate, error)

type RetentionGuard func(ctx context.Context, key string, action func(retain bool, tx pgx.Tx) error) error

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore

	ListExpiredArtifacts       ListFunc
	ListDownloadIntents        ListFunc
	ListClaimedArtifacts       ListFunc
	ListClaimedDatasets        ListFunc
	RecordArtifactPurged       MarkFunc
	RecordDownloadIntentPurged MarkFunc
	RecordDatasetLost          MarkFunc
	GuardArtifactRemoval       RetentionGuard
}

type Args struct{}

func (Args) Kind() string { return "object_reconcile" }

type Worker struct {
	river.WorkerDefaults[Args]
	Svc *Service
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[Args]) error {
	return w.Svc.Sweep(ctx)
}

func (s *Service) Sweep(ctx context.Context) error {

	if s.ListExpiredArtifacts == nil || s.ListDownloadIntents == nil || s.ListClaimedArtifacts == nil ||
		s.ListClaimedDatasets == nil || s.RecordArtifactPurged == nil || s.RecordDatasetLost == nil ||
		s.RecordDownloadIntentPurged == nil || s.GuardArtifactRemoval == nil {
		return errors.New("objreconcile: owner read/write functions not injected; refusing to sweep")
	}
	if s.Store == nil {
		return nil
	}
	if s.Pool == nil {
		return errors.New("objreconcile: pool not configured; refusing to sweep")
	}
	if err := s.purgeExpired(ctx); err != nil {
		return err
	}
	if err := s.checkArtifacts(ctx); err != nil {
		return err
	}
	if err := s.checkDatasets(ctx); err != nil {
		return err
	}
	return s.publishGauge(ctx)
}

func (s *Service) purgeExpired(ctx context.Context) error {
	_, artifactErr := PurgeExpired(ctx, s.Pool, s.Store, s.ListExpiredArtifacts,
		s.RecordArtifactPurged, s.GuardArtifactRemoval, batch)
	_, intentErr := PurgeExpired(ctx, s.Pool, s.Store, s.ListDownloadIntents,
		s.RecordDownloadIntentPurged, s.GuardArtifactRemoval, batch)
	return errors.Join(artifactErr, intentErr)
}

// PurgeExpired removes the object before marking its row purged: a row marked
// over bytes still in storage is a lie nothing corrects, while bytes removed
// under an unmarked row are simply retried and removed again next pass.
func PurgeExpired(
	ctx context.Context, pool *pgxpool.Pool, store ObjectStore,
	list ListFunc, mark MarkFunc, guard RetentionGuard, limit int32,
) (int, error) {
	if pool == nil || store == nil || list == nil || mark == nil {
		return 0, errors.New("objreconcile: retention sweep is missing its pool, store or owner functions")
	}
	rows, err := list(ctx, limit)
	if err != nil {
		return 0, err
	}
	purged := 0
	byKey := make(map[string][]Candidate, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := byKey[row.ObjectKey]; !ok {
			keys = append(keys, row.ObjectKey)
		}
		byKey[row.ObjectKey] = append(byKey[row.ObjectKey], row)
	}
	for _, key := range keys {
		group := byKey[key]
		var removeErr error
		action := func(retain bool, tx pgx.Tx) error {
			if !retain {
				removeErr = store.Remove(ctx, key)
				if removeErr != nil {
					return nil
				}
			}

			for _, row := range group {
				var err error
				if tx == nil {
					err = markPurged(ctx, pool, mark, row.ID)
				} else {
					err = mark(ctx, tx, row.ID)
				}
				if err != nil {
					return err
				}
				purged++
				slog.Info("artifact purged at retention", "artifact_id", pgconv.UUIDString(row.ID))
			}
			return nil
		}
		var guardErr error
		if guard == nil {
			guardErr = action(false, nil)
		} else {
			guardErr = guard(ctx, key, action)
		}
		if guardErr != nil {
			return purged, guardErr
		}
		if removeErr != nil {
			for _, row := range group {
				slog.Warn("expired object not removed; will retry",
					"artifact_id", pgconv.UUIDString(row.ID), "error", removeErr)
			}
		}
	}
	return purged, nil
}

func markPurged(ctx context.Context, pool *pgxpool.Pool, mark MarkFunc, artifactID pgtype.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := mark(ctx, tx, artifactID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) checkArtifacts(ctx context.Context) error {
	q := gen.New(s.Pool)
	checked := map[string]objectCheck{}
	rows, err := s.ListClaimedArtifacts(ctx, batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		confirmed, err := s.sight(ctx, q, checked, kindArtifact, row.ID, row.ObjectKey)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		if err := s.markLost(ctx, kindArtifact, row.ID, row.WorkspaceID, row.ObjectKey,
			s.RecordArtifactPurged,
			audit.ResourceArtifact); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) checkDatasets(ctx context.Context) error {
	q := gen.New(s.Pool)
	checked := map[string]objectCheck{}
	rows, err := s.ListClaimedDatasets(ctx, batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		confirmed, err := s.sight(ctx, q, checked, kindDataset, row.ID, row.ObjectKey)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		if err := s.markLost(ctx, kindDataset, row.ID, row.WorkspaceID, row.ObjectKey,
			s.RecordDatasetLost,
			audit.ResourceDataset); err != nil {
			return err
		}
	}
	return nil
}

type objectCheck struct {
	present bool
	err     error
}

func (s *Service) sight(
	ctx context.Context, q *gen.Queries, checked map[string]objectCheck,
	kind string, id pgtype.UUID, key string,
) (bool, error) {
	check, ok := checked[key]
	if !ok {
		check.present, check.err = s.Store.Exists(ctx, key)
		checked[key] = check
	}
	present, err := check.present, check.err
	if err != nil {
		slog.Warn("object existence check failed; not counting a sighting",
			"kind", kind, "id", pgconv.UUIDString(id), "error", err)
		return false, nil
	}
	if present {

		return false, q.ClearObjectSighting(ctx, gen.ClearObjectSightingParams{
			ResourceKind: kind, ResourceID: id,
		})
	}
	rounds, err := q.RecordObjectSighting(ctx, gen.RecordObjectSightingParams{
		ResourceKind: kind, ResourceID: id, ObjectKey: key,
	})
	if err != nil {
		return false, err
	}
	if rounds < actAfterRounds {
		slog.Warn("object missing behind a live row; waiting for a second round",
			"kind", kind, "id", pgconv.UUIDString(id), "object_key", key)
		return false, nil
	}
	return true, nil
}

func (s *Service) markLost(
	ctx context.Context, kind string, id, workspaceID pgtype.UUID, key string,
	mark MarkFunc, resourceType string,
) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	if err := mark(ctx, tx, id); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Workspace: workspaceID, Action: audit.ActionObjectMissing,
		ResourceType: resourceType, ResourceID: id,

		Metadata: map[string]any{"object_key": key, "rounds": actAfterRounds},
	}); err != nil {
		return err
	}

	if err := q.ClearObjectSighting(ctx, gen.ClearObjectSightingParams{
		ResourceKind: kind, ResourceID: id,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Error("stored object missing; the row no longer claims it exists",
		"kind", kind, "id", pgconv.UUIDString(id), "object_key", key)
	return nil
}

func (s *Service) publishGauge(ctx context.Context) error {
	rows, err := gen.New(s.Pool).CountPersistentObjectSightings(ctx, actAfterRounds)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		metrics.ObjectsMissing.WithLabelValues(row.ResourceKind).Set(float64(row.Sightings))
		seen[row.ResourceKind] = true
	}

	for _, kind := range []string{kindDataset, kindArtifact} {
		if !seen[kind] {
			metrics.ObjectsMissing.WithLabelValues(kind).Set(0)
		}
	}
	return nil
}
