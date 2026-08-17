-- Packaging (PACK-001/002/003/005, 0027). Every statement that touches user
-- content is workspace scoped (iron rule 3); the two lineage reads at the bottom
-- are the deliberate exception and say why.

-- name: FindReusableDownloadArtifact :one
-- The idempotency lookup of POST .../packaging. The four columns of 0027's
-- dedupe index identify a package; the other three predicates are the part no
-- index can express, which is why 0027 left this as a lookup rather than a
-- unique constraint: "already has one" also means the bytes are still there and
-- still servable.
SELECT da.artifact_id, da.skill_version_id, da.target, da.profile_version,
       da.packager_version, da.manifest_hash, da.includes_test_cases,
       a.file_name, a.size_bytes, a.content_hash, a.scan_status,
       a.expires_at, a.created_at,
       (SELECT count(*) FROM download_records dr WHERE dr.artifact_id = da.artifact_id)::bigint
           AS download_count
FROM download_artifacts da
JOIN artifacts a ON a.id = da.artifact_id
WHERE da.workspace_id = $1
  AND da.skill_version_id = $2
  AND da.target = $3
  AND da.packager_version = $4
  AND da.includes_test_cases = $5
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
RETURNING *;

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
WHERE object_key = $1 AND deleted_at IS NULL AND purged_at IS NULL;

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
