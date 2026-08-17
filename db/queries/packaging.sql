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
