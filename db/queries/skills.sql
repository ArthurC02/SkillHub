-- name: CreateSkill :one
INSERT INTO skills (workspace_id, name, summary, forked_from_skill_id, forked_from_version_id,
                    access_restriction, redistribution, category, category_source)
VALUES (@workspace_id, @name, @summary, @forked_from_skill_id, @forked_from_version_id,
        @access_restriction, @redistribution, sqlc.narg('category')::text, sqlc.narg('category_source')::text)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: LockSkill :one
SELECT * FROM skills
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: GetCatalogSkill :one
SELECT * FROM skills
WHERE id = @id AND workspace_id = ANY(@catalog_workspace_ids::uuid[]) AND deleted_at IS NULL;

-- name: SoftDeleteSkill :one
UPDATE skills SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSkillSummary :exec
UPDATE skills SET summary = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: SetSkillCategory :one
UPDATE skills
SET category = $3, category_source = $4, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: ListSkills :many
SELECT sqlc.embed(sk), ver.created_at AS verified_at, ver.source_id AS verified_source_id,
       COALESCE(ver.content_hash, '')::text AS newest_content_hash,
       count(*) OVER ()::bigint AS total_matches
FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.created_at, v.source_id, v.content_hash
    FROM skill_versions v
    WHERE v.skill_id = sk.id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
WHERE sk.workspace_id = @workspace_id AND sk.deleted_at IS NULL
ORDER BY sk.created_at DESC
LIMIT @row_limit::int OFFSET @row_offset::int;

-- name: ListForkedFromVersions :many
SELECT v.id AS version_id, v.content_hash, v.created_at,
       anc.id AS skill_id, anc.workspace_id, anc.name, anc.deleted_at, anc.takedown_at
FROM skill_versions v
JOIN skills anc ON anc.id = v.skill_id
WHERE v.id = ANY(@version_ids::uuid[]);

-- name: CountSkillVersions :one
SELECT count(*) FROM skill_versions
WHERE skill_id = $1;

-- name: GetSkillSource :one
SELECT * FROM skill_sources
WHERE id = $1 AND workspace_id = $2;

-- name: GetSkillEnrichment :one
SELECT summary, enriched_summary, task_examples, tags, limitations,
       enrichment_status, enrichment_model, enrichment_prompt_version
FROM search_documents
WHERE skill_id = $1 AND workspace_id = $2;

-- name: GetLiveSkillListingFacts :one
SELECT sk.redistribution, sk.category, sk.category_source, sk.curation_tier, sk.curated_version_id,
       ver.id AS latest_version_id, ver.created_at AS verified_at,
       COALESCE(ver.package_object_key, '')::text AS latest_package_object_key,
       COALESCE(ver.source_path, '')::text AS latest_source_path,
       COALESCE(cmp.capability, '')::text AS agent_capability,
       COALESCE(cmp.runtime, '')::text AS agent_runtime,
       COALESCE(cmp.runtime_image, '')::text AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at
FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at, v.package_object_key, v.source_path
    FROM skill_versions v
    WHERE v.skill_id = sk.id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
LEFT JOIN LATERAL (
    SELECT c.capability, c.runtime, c.runtime_image, c.measured_at
    FROM skill_runtime_compatibility c
    WHERE c.skill_version_id = ver.id
    ORDER BY c.measured_at DESC
    LIMIT 1
) cmp ON true
WHERE sk.id = $1 AND sk.deleted_at IS NULL AND sk.takedown_at IS NULL
FOR NO KEY UPDATE OF sk;

-- name: ListLiveSkillsForIndex :many
SELECT id, workspace_id, name, coalesce(summary, '')::text AS summary, redistribution
FROM skills
WHERE deleted_at IS NULL AND takedown_at IS NULL
ORDER BY id;

-- name: ListLiveSkillIDs :many
SELECT id FROM skills
WHERE id = ANY(sqlc.arg(skill_ids)::uuid[]) AND deleted_at IS NULL AND takedown_at IS NULL;

-- name: FindSkillsForGovernance :many
SELECT id, workspace_id, name, access_restriction, redistribution, takedown_at, takedown_reason
FROM skills
WHERE deleted_at IS NULL
  AND (id = sqlc.narg(skill_id)::uuid
       OR (sqlc.narg(skill_id)::uuid IS NULL AND name ILIKE '%' || @name_part::text || '%'))
ORDER BY created_at DESC, id
LIMIT @result_limit;

-- name: ListForkedSkills :many
SELECT f.forked_from_skill_id::uuid AS skill_id FROM skills f
WHERE f.forked_from_skill_id = ANY(@skill_ids::uuid[])
UNION
SELECT v.skill_id FROM skills f
JOIN skill_versions v ON v.id = f.forked_from_version_id
WHERE v.skill_id = ANY(@skill_ids::uuid[]);
