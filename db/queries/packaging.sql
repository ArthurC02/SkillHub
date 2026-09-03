-- Packaging (PACK-001/002/003/005, 0027). Every statement that touches user
-- content is workspace scoped (iron rule 3); the two lineage reads at the bottom
-- are the deliberate exception and say why.

-- name: FindReusableDownloadArtifact :one
-- The idempotency lookup of POST .../packaging. Mutable compatibility and
-- portable Test Case inputs can change the bytes for one Skill Version, so the
-- exact zip content hash is part of reuse identity.
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.version_number,
       -- 04 丙-42: the download row said which bytes, never which version, and
       -- never whether a newer one exists. Both halves are one join and one
       -- subquery away, and without them WS-002's "版本" is a uuid.
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
WHERE da.workspace_id = $1
  AND da.skill_version_id = $2
  AND da.target = $3
  AND da.packager_version = $4
  AND da.includes_test_cases = $5
  AND a.content_hash = $6
  AND a.scan_status = 'available'
  AND a.deleted_at IS NULL
  -- Added by 0028: bytes the retention sweep or the reconciler removed. Without
  -- this an artifact whose object is gone would be handed back as a duplicate and
  -- the caller would be sent to a download that answers 404.
  AND a.purged_at IS NULL
  AND a.expires_at > now()
ORDER BY a.created_at DESC
LIMIT 1;

-- name: CreateDownloadArtifactRow :one
-- The generic half. run_id is NULL, which 0004 reserved in as many words ("NULL
-- for packaging downloads"). scan_status keeps its 'quarantined' default: the
-- row exists before the object is servable, and the caller flips it in the same
-- transaction once the produced bytes have passed re-validation (ADR-003).
--
-- expires_at is passed in, never defaulted: PDM-006's 90 days is proposed and
-- not ratified, so the value is deployment configuration and a default here
-- would turn a proposal into schema.
INSERT INTO artifacts (
    workspace_id, run_id, kind, file_name, content_type,
    size_bytes, content_hash, object_key, expires_at
) VALUES ($1, NULL, 'download_package', $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CreateDownloadArtifactDetail :one
-- The packaging half (0027 4-a). The composite foreign key checks that the
-- artifact it hangs off is a download package in this same workspace.
INSERT INTO download_artifacts (
    artifact_id, workspace_id, skill_version_id, target,
    profile_version, packager_version, manifest_hash, includes_test_cases
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
-- 04 丙-42: the skill's highest version number, read in the same statement that
-- writes the row. A second round trip would be a second point in time, and the
-- one thing this number must not do is disagree with the row it is shown beside.
RETURNING *, (
    SELECT max(v2.version_number) FROM skill_versions v2
     WHERE v2.skill_id = (SELECT v1.skill_id FROM skill_versions v1
                           WHERE v1.id = download_artifacts.skill_version_id)
)::int AS latest_version_number;

-- name: MarkDownloadArtifactAvailable :exec
-- The quarantine release of ADR-003. Kind is in the predicate so this can never
-- publish a run output, and workspace_id so it can never publish another
-- tenant's (iron rule 3).
UPDATE artifacts SET scan_status = 'available'
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package';

-- The download surface (WS-002, WS-004, SEC-006). Every statement is workspace
-- scoped and none of them takes the workspace from a caller (iron rule 3).

-- name: ListDownloadArtifacts :many
-- GET /downloads: the workspace's packages, newest first.
--
-- Expired rows stay in the list on purpose — "it expired" and "it never existed"
-- are different answers to 02:WS-002 1, and dropping the row silently gives the
-- wrong one. Rows the OWNER deleted do not: 02:SEC-006 requires deleted content
-- to stop appearing in ordinary access surfaces, which is exactly what
-- deleted_at means and purged_at does not (0028).
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.skill_id,
       sv.version_number,
       -- 04 丙-42: the download row said which bytes, never which version, and
       -- never whether a newer one exists. Both halves are one join and one
       -- subquery away, and without them WS-002's "版本" is a uuid.
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at, a.purged_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
WHERE da.workspace_id = $1 AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, da.artifact_id;

-- name: GetDownloadArtifact :one
-- One row, for GET /downloads/{id}, the content stream and the delete.
--
-- It carries three things the JSON shape does not: the object key, and the
-- skill's two independent locks. Serving bytes re-checks those locks on every
-- request rather than trusting the verdict packaging reached — a hold applied
-- after the package was built has to stop the copy that already exists from
-- going out (packaging-design §7.1, the argument against pre-signed URLs).
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       sv.skill_id,
       sv.version_number,
       -- 04 丙-42: the download row said which bytes, never which version, and
       -- never whether a newer one exists. Both halves are one join and one
       -- subquery away, and without them WS-002's "版本" is a uuid.
       (SELECT max(v2.version_number) FROM skill_versions v2
         WHERE v2.skill_id = sv.skill_id)::int AS latest_version_number,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status, a.object_key,
       a.expires_at, a.created_at, a.purged_at,
       sk.access_restriction, sk.redistribution,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
JOIN skill_versions sv ON sv.id = da.skill_version_id
JOIN skills sk ON sk.id = sv.skill_id
WHERE da.workspace_id = $1 AND da.artifact_id = $2 AND a.deleted_at IS NULL;

-- name: InsertDownloadRecord :exec
-- WS-004, append only (0027). Written in the same transaction as the audit event
-- so a download cannot be in one record and missing from the other (iron rule 9),
-- and kept in a separate table from it because their retention and their
-- visibility differ (packaging-design §7.2).
--
-- No lock and no counter to increment: the count is COUNT(*) over these rows, so
-- concurrent downloads of one artifact are two inserts that cannot race.
INSERT INTO download_records (workspace_id, artifact_id, actor_user_id)
VALUES ($1, $2, $3);

-- name: ListDownloadRecordsForArtifact :many
-- WS-004's own words: "誰、何時、哪一筆 artifact、哪一個 profile". The list above
-- answers the last two and a count; this answers the first two, one row per
-- download, which is what the work item asks for and what an aggregate cannot
-- give.
--
-- Deliberately NOT the audit event (CORE-008). This is the product feature the
-- owner reads, and it may be deleted with the account; the audit row is the
-- compliance record with its own retention and its own visibility. Same download,
-- two rows, and neither substitutes for the other (packaging-design §7.2).
--
-- The actor is served as a display name rather than as a user id: on a personal
-- workspace it is always the owner, and an id would be an identifier the reader
-- cannot resolve. LEFT JOIN because a purged account's rows survive
-- de-identified (PDM-006 §6.1) and "somebody, at this time" is still true.
SELECT dr.downloaded_at, dr.actor_user_id, u.display_name
FROM download_records dr
JOIN download_artifacts da ON da.artifact_id = dr.artifact_id
LEFT JOIN users u ON u.id = dr.actor_user_id
WHERE dr.workspace_id = @workspace_id AND dr.artifact_id = @artifact_id
  AND da.workspace_id = @workspace_id
ORDER BY dr.downloaded_at DESC;

-- name: SoftDeleteDownloadArtifact :one
-- DELETE /downloads/{id}. Soft, although the OpenAPI prose once said the row
-- goes: download_records has a foreign key onto download_artifacts and those
-- records outlive the file by design (WS-004), and download_artifacts carries
-- 0027's immutability trigger. So the row stays and stops being visible, which is
-- what 02:SEC-006 actually asks for.
--
-- Returns nothing when there is nothing to delete, which is what makes the
-- endpoint idempotent: a repeat of a delete that worked is not a failure.
UPDATE artifacts SET deleted_at = now()
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package' AND deleted_at IS NULL
RETURNING id, object_key, purged_at;

-- name: GetDownloadArtifactForDelete :one
-- Reads the immutable object key before the session advisory lock is acquired.
-- Delete rechecks ownership and liveness with SoftDeleteDownloadArtifact after
-- obtaining the lock; this first read reveals no more than the scoped delete.
SELECT id, object_key, purged_at FROM artifacts
WHERE id = $1 AND workspace_id = $2 AND kind = 'download_package' AND deleted_at IS NULL;

-- name: DeleteWorkspaceDownloadRecords :execrows
-- CORE-007, first of the three statements the account purge needs from this
-- context, and the order between them is the foreign keys' and not a preference.
--
-- 0027 hung download_records on download_artifacts and download_artifacts on
-- artifacts, both with composite keys and neither with ON DELETE, so packaging's
-- purge step deleting only the `artifacts` row (governance.sql's
-- DeleteWorkspaceDownloadArtifacts) raised 23503 on any workspace that had ever
-- produced one package — and the whole account purge rolled back with it, every
-- sweep, forever. See delivery/purge.go for why that stayed invisible.
--
-- Not ON DELETE CASCADE, deliberately: a cascade would also fire on a delete
-- nobody meant, and these two tables are frozen by 0027 precisely because "you
-- downloaded this on that date" is not editable state. The delete happens here,
-- in daylight, under the purge flag, or it does not happen.
--
-- Requires SET LOCAL skillhub.purge = 'on' in the same transaction (0013);
-- identity/purge.go already sets it before any context's step runs.
DELETE FROM download_records WHERE workspace_id = $1;

-- name: DeleteWorkspaceDownloadArtifactDetails :execrows
-- The middle row of the same three. Named "…Details" and not
-- "DeleteWorkspaceDownloadArtifacts" because sqlc's namespace is flat and that
-- name is already taken — by the statement in governance.sql that deletes the
-- `artifacts` parent, which is the confusion that let the missing statement look
-- present for as long as it did.
--
-- Same purge flag as the record above (0027's trigger on this table too).
DELETE FROM download_artifacts WHERE workspace_id = $1;

-- name: CountArtifactsSharingObject :one
-- Whether anybody else still needs these bytes, asked after the row above is
-- already soft-deleted so it does not count itself.
--
-- Object keys are content addressed, so two rows CAN name one object. A download
-- package's manifest carries its own version ids, which makes a collision between
-- workspaces close to impossible — but "close to impossible" is not the standard
-- for an unrecoverable delete of somebody else's file, and governance.sql already
-- spares package objects for the same reason.
SELECT count(*)::bigint FROM artifacts
WHERE object_key = $1 AND deleted_at IS NULL AND purged_at IS NULL
  AND expires_at > now();

-- name: ListTestCasesForSkill :many
-- The PACK-005 candidates: this skill's test cases in the caller's workspace.
-- Whether any of them may travel is decided in Go, not here — the criterion is
-- curation, not existence.
SELECT * FROM test_cases
WHERE skill_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: ListSuggestionsAppliedToVersion :many
-- PACK-003's third provenance path, read backwards. There is no
-- `derived_from_evaluation_id` column and one is deliberately not being added
-- (m3/evaluation-design §5.3), so "which suggestions built this version" is the
-- reverse lookup on applied_skill_version_id.
--
-- Only the two columns the manifest may carry: `problem`, `proposed_content`,
-- `expected_impact` and the evidence excerpts are model-written prose quoting a
-- Run's private inputs, and a package is not where they go (iron rule 11).
SELECT evaluation_id, category, target_path
FROM evaluation_suggestions
WHERE applied_skill_version_id = $1 AND workspace_id = $2
ORDER BY created_at, id;

-- name: GetPreviousSkillVersion :one
-- The version an improvement was built on top of. Workspace scoped: it is the
-- caller's own skill either way.
SELECT id, skill_id, version_number FROM skill_versions
WHERE skill_id = $1 AND workspace_id = $2 AND version_number < $3
ORDER BY version_number DESC
LIMIT 1;

-- The two statements below are deliberately NOT workspace scoped, and that needs
-- saying rather than being noticed later.
--
-- A fork's upstream lives in another workspace by definition (registry.Fork
-- leaves source_id NULL precisely because the skill_sources row belongs to the
-- origin workspace). DISC-003 clause 5 requires a packaged version to be
-- traceable to its ORIGINAL source, so walking out of the caller's workspace is
-- the requirement, not a leak of it.
--
-- What they return is bounded to that: lineage identifiers and the import facts
-- the public catalogue already shows (source type, URL, ref, fetch time, the
-- fetched artefact's hash). No name, no summary, no package bytes, nothing that
-- says whether the upstream still exists as anything a reader could open.

-- name: GetVersionLineage :one
SELECT sv.id, sv.skill_id, sv.version_number, sv.source_id,
       sk.forked_from_skill_id, sk.forked_from_version_id
FROM skill_versions sv
JOIN skills sk ON sk.id = sv.skill_id
WHERE sv.id = $1;

-- name: GetOldestSkillVersion :one
-- Where a skill's own lineage starts. A fork's first version has source_id NULL,
-- which is what makes the walk continue up to the next hop instead of stopping.
SELECT id, skill_id, version_number, source_id
FROM skill_versions
WHERE skill_id = $1
ORDER BY version_number
LIMIT 1;

-- name: GetLineageSource :one
SELECT source_type, source_url, source_ref, content_hash, fetched_at
FROM skill_sources
WHERE id = $1;
-- name: LockDownloadObjectKey :exec
-- Serialises creation and deletion of rows that share one content-addressed
-- download object. Transaction-scoped so every exit releases it.
SELECT pg_advisory_xact_lock(hashtextextended(@lock_key::text, 0));

-- name: CreateDownloadCleanupIntent :one
INSERT INTO download_object_cleanup_intents (workspace_id, object_key)
VALUES (@workspace_id, @object_key)
ON CONFLICT (object_key) DO UPDATE
SET workspace_id = excluded.workspace_id,
    not_before = now() + interval '1 hour', attempted_at = NULL
RETURNING *;

-- name: DeleteDownloadCleanupIntent :exec
DELETE FROM download_object_cleanup_intents
WHERE object_key = @object_key AND workspace_id = @workspace_id;

-- name: LockPackagingWorkspaceObjects :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: LockPackagingWorkspaceObjectsSession :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockPackagingWorkspaceObjectsSession :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: LockDownloadObjectKeySession :exec
-- Delete must keep this lock across its transaction commit and the following
-- object-store removal. It is always paired with UnlockDownloadObjectKeySession
-- on the same acquired connection.
SELECT pg_advisory_lock(hashtextextended(@lock_key::text, 0));

-- name: UnlockDownloadObjectKeySession :one
SELECT pg_advisory_unlock(hashtextextended(@lock_key::text, 0));
