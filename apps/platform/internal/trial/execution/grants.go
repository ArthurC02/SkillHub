package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const grantSlack = 5 * time.Minute

func artifactObjectKey(runID, runAttemptID string) string {
	return fmt.Sprintf("run-artifacts/%s/%s/artifacts.tar", runID, runAttemptID)
}

func artifactUploadIntent(a gen.RunAttempt) gen.RememberRunArtifactUploadIntentParams {
	return gen.RememberRunArtifactUploadIntentParams{
		RunAttemptID: a.ID, WorkspaceID: a.WorkspaceID,
		NotBefore: pgtype.Timestamptz{Time: a.ObjectGrantsExpireAt.Time.Add(runArtifactRetention), Valid: true},
		ObjectKey: artifactObjectKey(pgconv.UUIDString(a.RunID), pgconv.UUIDString(a.ID)),
	}
}

type runInputs struct {
	version       VersionFacts
	datasets      []testlab.DatasetRef
	skillWithheld bool
}

func (s *Service) grantsFor(
	ctx context.Context, run gen.Run, attempt gen.RunAttempt, inputs runInputs, ttl time.Duration,
) (grants []ObjectGrant, datasetKeys []string, err error) {
	version, refs := inputs.version, inputs.datasets
	if err := s.requireTestLab(); err != nil {
		return nil, nil, unbuildable(err)
	}
	needsPackage := !inputs.skillWithheld
	if needsPackage && version.PackageObjectKey == "" {
		return nil, nil, unbuildable(errSkillPackageMissing)
	}
	store := s.Store
	datasetKeys = make([]string, len(refs))
	if store == nil {
		if needsPackage || len(refs) > 0 {
			return nil, nil, unbuildable(errors.New("no object store is configured; this run's inputs cannot be granted"))
		}
		if err := s.recordObjectGrantExpiry(ctx, attempt, objectGrantsExpiredOnArrival()); err != nil {
			return nil, nil, fmt.Errorf("record empty object grant expiry: %w", err)
		}
		return nil, datasetKeys, nil
	}
	expires := time.Now().UTC().Add(ttl)
	grants = make([]ObjectGrant, 0, len(refs)+2)

	if needsPackage {
		url, err := store.PresignGet(ctx, version.PackageObjectKey, ttl)
		if err != nil {
			return nil, nil, fmt.Errorf("grant skill package: %w", err)
		}
		grants = append(grants, ObjectGrant{
			Purpose: "skill_package", ObjectKey: version.PackageObjectKey,
			Access: "read", URL: url, ExpiresAt: expires,
		})
	}

	for i, ref := range refs {
		grant, err := s.datasetGrant(ctx, run.WorkspaceID, ref, ttl)
		if err != nil {
			return nil, nil, err
		}
		datasetKeys[i] = grant.ObjectKey
		grants = append(grants, grant)
	}

	key := artifactObjectKey(pgconv.UUIDString(run.ID), pgconv.UUIDString(attempt.ID))
	url, err := store.PresignPut(ctx, key, ttl)
	if err != nil {
		return nil, nil, fmt.Errorf("grant artifact upload: %w", err)
	}
	grants = append(grants, ObjectGrant{
		Purpose: "artifact_upload", ObjectKey: key,
		Access: "write", URL: url, ExpiresAt: expires,
	})

	expires = time.Now().UTC().Add(ttl)
	for i := range grants {
		grants[i].ExpiresAt = expires
	}
	if err := s.recordObjectGrantExpiry(ctx, attempt, expires); err != nil {
		return nil, nil, fmt.Errorf("record object grant expiry: %w", err)
	}
	return grants, datasetKeys, nil
}

func (s *Service) datasetGrant(ctx context.Context, workspaceID pgtype.UUID, ref testlab.DatasetRef, ttl time.Duration) (ObjectGrant, error) {
	var id pgtype.UUID
	if err := id.Scan(ref.DatasetID); err != nil {
		return ObjectGrant{}, unbuildable(fmt.Errorf("grant dataset %s: %w", ref.FileName, err))
	}
	dataset, err := s.TestLab.ReadDataset(ctx, workspaceID, id)
	if err != nil {
		return ObjectGrant{}, unbuildableWhenGone(fmt.Errorf("dataset %s is no longer available for this run: %w", ref.FileName, err))
	}
	url, err := s.Store.PresignGet(ctx, dataset.ObjectKey, ttl)
	if err != nil {
		return ObjectGrant{}, fmt.Errorf("grant dataset %s: %w", ref.FileName, err)
	}
	return ObjectGrant{Purpose: "dataset", ObjectKey: dataset.ObjectKey, Access: "read", URL: url}, nil
}
