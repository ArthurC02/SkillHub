package run

// SBX-008: the short-lived authorizations one attempt is dispatched with.
//
// The execution plane holds no database connection and no storage credential
// (iron rule 2). Everything it is allowed to move is named here, one object and
// one direction at a time, with an expiry: the skill package it must unpack, the
// dataset files the test case froze, and the single key it may write its output
// to. Nothing else in the bucket is reachable with any of them.
//
// The URLs are secret material. They are built at dispatch, travel inside the
// RunRequest, and are never logged, traced or shown in the permission summary
// (iron rule 11) - the summary discloses *what* a run may touch, and a
// pre-signed URL is the touching, not the description of it.

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

// grantSlack is how much longer than the run's hard wall clock a grant lives. A
// grant that expires while the sandbox is still working would surface as an
// unexplainable download failure; the platform's own deadline is the one that is
// supposed to stop a run.
const grantSlack = 5 * time.Minute

// artifactObjectKey is where one attempt's collected output lands. Keyed by the
// permanent platform ids (iron rule 10), so a retry writes to its own key and a
// superseded attempt's output is never overwritten.
func artifactObjectKey(runID, runAttemptID string) string {
	return fmt.Sprintf("run-artifacts/%s/%s/artifacts.tar", runID, runAttemptID)
}

// grantsFor mints every object authorization one attempt needs.
//
// Fail-closed: a package or dataset that cannot be granted stops the dispatch
// rather than producing a sandbox that silently runs without its inputs. A run
// whose skill package is unreachable has not been executed, and reporting it as
// executed-and-failed would blame the skill for the platform's problem.
//
// datasetKeys is aligned with refs, so the caller can pair each frozen dataset
// reference with the object the grant addresses without joining two lists.
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
		updated, err := s.queries().SetRunAttemptObjectGrantsExpiry(ctx, gen.SetRunAttemptObjectGrantsExpiryParams{
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(-2 * time.Minute), Valid: true}, ID: attempt.ID, WorkspaceID: run.WorkspaceID,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("record empty object grant expiry: %w", err)
		}
		if updated != 1 {
			return nil, nil, errors.New("record empty object grant expiry: attempt not found in workspace")
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
		// Workspace scoped like every other read (iron rule 3). A dataset deleted
		// after the snapshot was frozen is gone for good: the snapshot is
		// immutable (iron rule 4) but the bytes are not, and a run that cannot
		// read what it was asked to read must say so rather than run without it.
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

	// One write grant, one key: the attempt's collected output as a single
	// archive (see collectArtifacts in apps/sandbox). Pre-signing is
	// per-object, and the platform cannot know the file names before the
	// workload has produced them, so a single archive key is what one grant can
	// actually authorize.
	key := artifactObjectKey(pgconv.UUIDString(run.ID), pgconv.UUIDString(attempt.ID))
	url, err := store.PresignPut(ctx, key, ttl)
	if err != nil {
		return nil, nil, fmt.Errorf("grant artifact upload: %w", err)
	}
	grants = append(grants, ObjectGrant{
		Purpose: "artifact_upload", ObjectKey: key,
		Access: "write", URL: url, ExpiresAt: expires,
	})
	// Presigned URLs count their TTL from each signer call, not from the time
	// above. Nothing has been handed to a provider yet, and the attempt's
	// fail-closed infinity default covers a crash here. Record an upper bound
	// after the final signature and use that same visible deadline for every grant.
	expires = time.Now().UTC().Add(ttl)
	for i := range grants {
		grants[i].ExpiresAt = expires
	}
	updated, err := s.queries().SetRunAttemptObjectGrantsExpiry(ctx, gen.SetRunAttemptObjectGrantsExpiryParams{
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}, ID: attempt.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("record object grant expiry: %w", err)
	}
	if updated != 1 {
		return nil, nil, errors.New("record object grant expiry: attempt not found in workspace")
	}
	return grants, datasetKeys, nil
}
