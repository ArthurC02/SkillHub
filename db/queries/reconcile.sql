-- Storage reconciliation (SEC-006 retention, 04 丙-9 object existence, 0028).
--
-- Two directions of one question — does what the database says about stored
-- objects match what storage has. Retention says bytes should be gone and this
-- makes them gone; the existence sweep finds bytes that are already gone and
-- stops the rows claiming otherwise.
--
-- None of these statements is workspace scoped, and that is not an iron rule 3
-- exception: they are the platform's own storage bookkeeping, they take no
-- caller input, and none of them is run on a user's behalf. Every one of them is
-- keyed by a row id the sweep itself read.

-- name: ListArtifactsPastRetention :many
-- The retention worklist: download packages whose expiry has passed and whose
-- bytes are still there. Bounded, because a sweep is not a migration.
--
-- Only download packages, and it stays that way. Run outputs live in the same
-- physical table but belong to the `run` context, and one statement cannot have
-- two owners: widening this predicate would make packaging's read return run's
-- rows and packaging's UPDATE write them, which is the cross-context write
-- db/query-owners.yaml exists to refuse. ListRunOutputsPastRetention below is
-- the same worklist for the other owner.
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'download_package'
      AND purged_at IS NULL
	  AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET retention_attempted_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: MarkArtifactPurged :exec
-- The bytes are gone and the row stays readable. Idempotent by predicate, which
-- is what lets the whole sweep be re-run safely (iron rule 9).
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'artifact' AND resource_id = $1
)
UPDATE artifacts SET purged_at = now()
WHERE id = $1 AND kind = 'download_package' AND purged_at IS NULL;

-- name: ListRunOutputsPastRetention :many
-- The same worklist for run's half of `artifacts` (PDM-006 §6, consent §3: 30
-- days). Deliberately a second statement rather than a wider predicate on the
-- one above — see that comment for why the owner split forces it.
--
-- The cutoff is the row's own `expires_at` and never a window handed in by the
-- caller. That column is what InsertRunArtifact wrote, what
-- ListReadableRunArtifacts serves from and what CountUnreadableRunArtifacts
-- counts against; a sweep taking its deadline from somewhere else would be a
-- second definition of the same date, and the first thing a mismatch does is
-- delete rows another statement still calls readable.
--
-- `deleted_at IS NULL` for the reason the download sweep has it, plus one that
-- is specific to run outputs: the user's own delete already removed the object,
-- and it did so behind a shared-key count this sweep does not have. Sweeping a
-- deleted row would be removing bytes with that guard switched off.
--
-- Rows CAN share an object_key — one attempt's manifest is many rows over one
-- archive — but they were written by one settle and expire together, so the pass
-- that removes the object is the pass that marks all of them.
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'run_output'
      AND purged_at IS NULL
	  AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET retention_attempted_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: MarkRunOutputPurged :exec
-- run's own copy of MarkArtifactPurged, because that one is packaging's and a
-- cross-context write is refused. `kind = 'run_output'` is in the predicate for
-- the reason SoftDeleteRunArtifact has it: a statement that could reach any kind
-- is a statement that could destroy the wrong one. Idempotent by predicate.
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'artifact' AND resource_id = $1
)
UPDATE artifacts SET purged_at = now()
WHERE id = $1 AND kind = 'run_output' AND purged_at IS NULL;

-- name: ListArtifactsClaimingObject :many
-- What the download surface currently promises is downloadable. Anything the
-- endpoint would refuse anyway is left out — an expired or purged row makes no
-- claim about storage, so a missing object under it is not a discrepancy.
WITH candidates AS (
    SELECT id FROM artifacts
    WHERE kind = 'download_package'
      AND scan_status = 'available'
      AND deleted_at IS NULL
      AND purged_at IS NULL
	  AND expires_at > now()
	  AND (reconcile_checked_at IS NULL OR reconcile_checked_at < now() - interval '15 minutes')
	ORDER BY reconcile_checked_at NULLS FIRST, reconcile_checked_at, created_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE artifacts a SET reconcile_checked_at = now()
FROM candidates c WHERE a.id = c.id
RETURNING a.id, a.workspace_id, a.object_key;

-- name: ListDatasetsClaimingObject :many
-- The other half of 丙-9. RunInputsStillAvailable answers from deleted_at and
-- expires_at alone, so exactly these rows are the ones it counts as available.
WITH candidates AS (
    SELECT id FROM datasets
    WHERE deleted_at IS NULL
      AND purged_at IS NULL
      AND expires_at > now()
	  AND (reconcile_checked_at IS NULL OR reconcile_checked_at < now() - interval '15 minutes')
    ORDER BY reconcile_checked_at NULLS FIRST, reconcile_checked_at, created_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE datasets d SET reconcile_checked_at = now()
FROM candidates c WHERE d.id = c.id
RETURNING d.id, d.workspace_id, d.object_key;

-- name: ListDatasetsPastRetention :many
-- The third worklist in this file, and the one that should have been the first:
-- 0004 built `datasets_expires_at_idx` for a "retention sweep" in the same
-- breath as the comment that names it, and the sweep was never written. So the
-- column, the index and the sentence explaining the design were all here, and
-- the part that runs was not -- the same four-parts-minus-one shape as the audit
-- events and the run outputs, and the third row of the consent table to be
-- caught in it (04 丙-64).
--
-- The user is told 90 days before they upload (`retention_days` on the upload
-- screen, testlab.DatasetRetention) and again in the consent form. Until this
-- statement existed the only DELETE that ever reached `datasets` was account
-- deletion, so a participant who never closed their account kept every file
-- they ever uploaded, forever, against a number the screen had already quoted
-- them.
--
-- Live rows enter when their retention window expires. Soft-deleted rows enter
-- until purged_at confirms their object was removed; this is the durable retry
-- path for an object-store failure after DeleteDataset commits.
WITH candidates AS (
    SELECT id FROM datasets
    WHERE purged_at IS NULL
      AND (deleted_at IS NOT NULL OR expires_at <= now())
	  AND (retention_attempted_at IS NULL OR retention_attempted_at < now() - interval '15 minutes')
    ORDER BY retention_attempted_at NULLS FIRST, retention_attempted_at, expires_at, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE datasets d SET retention_attempted_at = now()
FROM candidates c WHERE d.id = c.id
RETURNING d.id, d.workspace_id, d.object_key;

-- name: MarkDatasetPurged :exec
-- deleted_at hides the row; purged_at records that object cleanup finished.
-- Separate from MarkDatasetObjectLost because the two operations describe
-- different evidence even though both end with no bytes. Idempotent by
-- predicate, which is what lets the sweep be re-run safely (iron rule 9).
WITH cleared_sighting AS (
    DELETE FROM object_reconcile_sightings
    WHERE resource_kind = 'dataset' AND resource_id = $1
)
UPDATE datasets SET deleted_at = coalesce(deleted_at, now()), purged_at = now()
WHERE id = $1 AND purged_at IS NULL;

-- name: ListDatasetCleanupIntents :many
-- Failed or interrupted uploads have no dataset row to carry cleanup state, so
-- their pre-written intents form a small independent worklist. The one-hour
-- floor keeps a slow but live upload from racing its own cleanup.
WITH candidates AS (
    SELECT id FROM dataset_object_cleanup_intents
    WHERE not_before <= now()
	  AND (attempted_at IS NULL OR attempted_at < now() - interval '15 minutes')
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE dataset_object_cleanup_intents i SET attempted_at = now()
FROM candidates c WHERE i.id = c.id
RETURNING i.id, i.workspace_id, i.object_key;

-- name: MarkDatasetCleanupIntentPurged :exec
DELETE FROM dataset_object_cleanup_intents WHERE id = $1;

-- name: ListDownloadCleanupIntents :many
WITH candidates AS (
    SELECT id FROM download_object_cleanup_intents
    WHERE not_before <= now()
      AND (attempted_at IS NULL OR attempted_at < now() - interval '15 minutes')
    ORDER BY attempted_at NULLS FIRST, attempted_at, not_before, id
    LIMIT $1 FOR UPDATE SKIP LOCKED
)
UPDATE download_object_cleanup_intents i SET attempted_at = now()
FROM candidates c WHERE i.id = c.id
RETURNING i.id, i.workspace_id, i.object_key;

-- name: MarkDownloadCleanupIntentPurged :exec
DELETE FROM download_object_cleanup_intents WHERE id = $1;

-- name: LockDatasetObjectKey :exec
SELECT pg_advisory_xact_lock(hashtextextended('dataset-object:' || @object_key::text, 0));

-- name: CountLiveDatasetsSharingObject :one
SELECT count(*) FROM datasets
WHERE object_key = @object_key AND deleted_at IS NULL AND purged_at IS NULL
  AND expires_at > now();

-- name: RecordObjectSighting :one
-- One round found the object missing. Returns the consecutive-round count, which
-- is the threshold the caller acts on — never on the first round, because an
-- object store answering 404 during a write or a transient fault must not cost
-- somebody their file.
INSERT INTO object_reconcile_sightings (resource_kind, resource_id, object_key)
VALUES ($1, $2, $3)
ON CONFLICT (resource_kind, resource_id) DO UPDATE
SET rounds = object_reconcile_sightings.rounds + 1, last_seen_at = now()
RETURNING rounds;

-- name: ClearObjectSighting :exec
-- The object came back, or the row has now been marked and there is nothing left
-- to count. Deleting is what makes `rounds` consecutive rather than cumulative
-- (same bookkeeping as 0021).
DELETE FROM object_reconcile_sightings
WHERE resource_kind = $1 AND resource_id = $2;

-- name: CountPersistentObjectSightings :many
-- The gauge behind "is content still missing", by kind. A gauge and not a
-- counter for the reason 0021's does not work as one: the question is whether it
-- is still true now.
SELECT resource_kind, count(*)::bigint AS sightings
FROM object_reconcile_sightings
WHERE rounds >= $1
GROUP BY resource_kind;

-- name: MarkDatasetObjectLost :exec
-- The row stops claiming a file that is not there. Mark it purged too: there are
-- no bytes for the retention sweep to remove.
UPDATE datasets SET deleted_at = coalesce(deleted_at, now()), purged_at = now()
WHERE id = $1 AND purged_at IS NULL;
