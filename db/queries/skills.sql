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
SELECT sk.* FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
WHERE sk.id = $1 AND sk.deleted_at IS NULL;

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
    JOIN workspaces w ON w.id = anc.workspace_id AND w.is_catalog
    JOIN skill_versions ancv ON ancv.id = sk.forked_from_version_id AND ancv.skill_id = anc.id
    WHERE ver.source_id IS NULL
      AND anc.id = sk.forked_from_skill_id
      AND anc.deleted_at IS NULL AND anc.takedown_at IS NULL
      AND ancv.content_hash = ver.content_hash
) inh ON true
WHERE sk.workspace_id = $1 AND sk.deleted_at IS NULL
ORDER BY sk.created_at DESC
LIMIT $2 OFFSET $3;

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
