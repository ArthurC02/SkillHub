package run

import (
	"context"
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

func (s *Service) grantsFor(
	ctx context.Context, run gen.Run, attempt gen.RunAttempt,
	version VersionFacts, refs []testlab.DatasetRef, ttl time.Duration,
) (grants []ObjectGrant, datasetKeys []string, err error) {
	if err := s.requireTestLab(); err != nil {
		return nil, nil, err
	}
	store := s.Store
	datasetKeys = make([]string, len(refs))
	if store == nil {
		if version.PackageObjectKey != "" || len(refs) > 0 {
			return nil, nil, fmt.Errorf("no object store is configured; this run's inputs cannot be granted")
		}
		if err := s.recordObjectGrantExpiry(ctx, attempt, objectGrantsExpiredOnArrival()); err != nil {
			return nil, nil, fmt.Errorf("record empty object grant expiry: %w", err)
		}
		return nil, datasetKeys, nil
	}
	expires := time.Now().UTC().Add(ttl)
	grants = make([]ObjectGrant, 0, len(refs)+2)

	if version.PackageObjectKey != "" {
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
		var id pgtype.UUID
		if err := id.Scan(ref.DatasetID); err != nil {
			return nil, nil, fmt.Errorf("grant dataset %s: %w", ref.FileName, err)
		}

		dataset, err := s.TestLab.ReadDataset(ctx, run.WorkspaceID, id)
		if err != nil {
			return nil, nil, fmt.Errorf("dataset %s is no longer available for this run: %w", ref.FileName, err)
		}
		url, err := store.PresignGet(ctx, dataset.ObjectKey, ttl)
		if err != nil {
			return nil, nil, fmt.Errorf("grant dataset %s: %w", ref.FileName, err)
		}
		datasetKeys[i] = dataset.ObjectKey
		grants = append(grants, ObjectGrant{
			Purpose: "dataset", ObjectKey: dataset.ObjectKey,
			Access: "read", URL: url, ExpiresAt: expires,
		})
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
