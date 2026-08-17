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
-- Only download packages. Run outputs carry their own expiry (0004: 30 days) and
-- nothing sweeps them yet; widening this predicate is how that gets fixed, but
-- doing it here would mean deleting run evidence as a side effect of shipping
-- the download surface.
SELECT id, workspace_id, object_key FROM artifacts
WHERE kind = 'download_package'
  AND expires_at <= now()
  AND purged_at IS NULL
  AND deleted_at IS NULL
ORDER BY expires_at
LIMIT $1;

-- name: MarkArtifactPurged :exec
-- The bytes are gone and the row stays readable. Idempotent by predicate, which
-- is what lets the whole sweep be re-run safely (iron rule 9).
UPDATE artifacts SET purged_at = now()
WHERE id = $1 AND purged_at IS NULL;

-- name: ListArtifactsClaimingObject :many
-- What the download surface currently promises is downloadable. Anything the
-- endpoint would refuse anyway is left out — an expired or purged row makes no
-- claim about storage, so a missing object under it is not a discrepancy.
SELECT id, workspace_id, object_key FROM artifacts
WHERE kind = 'download_package'
  AND scan_status = 'available'
  AND deleted_at IS NULL
  AND purged_at IS NULL
  AND expires_at > now()
ORDER BY created_at
LIMIT $1;

-- name: ListDatasetsClaimingObject :many
-- The other half of 丙-9. RunInputsStillAvailable answers from deleted_at and
-- expires_at alone, so exactly these rows are the ones it counts as available.
SELECT id, workspace_id, object_key FROM datasets
WHERE deleted_at IS NULL
  AND expires_at > now()
ORDER BY created_at
LIMIT $1;

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
-- The row stops claiming a file that is not there. deleted_at and not a new
-- column: every read path that cares — ListDatasets, the run input grants,
-- RunInputsStillAvailable — already filters on it, so correcting the optimistic
-- upper bound of 丙-9 needs no change on any read path at all.
UPDATE datasets SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL;
