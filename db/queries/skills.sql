-- name: CreateSkill :one
INSERT INTO skills (workspace_id, name, summary, forked_from_skill_id, forked_from_version_id,
                    access_restriction, redistribution, category, category_source)
VALUES ($1, $2, $3, $4, $5, $6, coalesce(sqlc.narg('redistribution')::text, 'unknown'),
        sqlc.narg('category')::text, sqlc.narg('category_source')::text)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

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
       inh.skill_id AS inherited_from_skill_id,
       COALESCE(inh.name, '') AS inherited_from_name,
       inh.created_at AS inherited_verified_at,
       count(*) OVER ()::bigint AS total_matches
FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.created_at, v.source_id, v.content_hash
    FROM skill_versions v
    WHERE v.skill_id = sk.id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
LEFT JOIN LATERAL (
    SELECT anc.id AS skill_id, anc.name, ancv.created_at
    FROM skills anc
    JOIN skill_versions ancv ON ancv.id = sk.forked_from_version_id AND ancv.skill_id = anc.id
    WHERE ver.source_id IS NULL
      AND anc.id = sk.forked_from_skill_id
      AND anc.workspace_id = ANY(@catalog_workspace_ids::uuid[])
      AND anc.deleted_at IS NULL AND anc.takedown_at IS NULL
      AND ancv.content_hash = ver.content_hash
) inh ON true
WHERE sk.workspace_id = @workspace_id AND sk.deleted_at IS NULL
ORDER BY sk.created_at DESC
LIMIT @row_limit::int OFFSET @row_offset::int;

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
       COALESCE(cmp.capability, 'unverified')::text AS agent_capability,
       COALESCE(cmp.runtime, 'unverified')::text AS agent_runtime,
       COALESCE(cmp.runtime_image, '')::text AS agent_runtime_image,
       cmp.measured_at AS agent_measured_at
FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.id, v.created_at, v.package_object_key
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
