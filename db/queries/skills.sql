-- Every read here is workspace scoped (iron rule 3). The caller resolves workspace_id
-- from the session, never from request input.

-- name: CreateSkill :one
INSERT INTO skills (workspace_id, name, summary, forked_from_skill_id, forked_from_version_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: GetCatalogSkill :one
-- The only read that reaches outside the caller's own workspace (WS-001 fork of
-- public content). The scope is baked into the statement — catalog workspaces
-- only (0010) — and deliberately takes no workspace argument: a widened scope
-- that a caller could name is exactly what iron rule 3 forbids. Private skills
-- of other workspaces do not match, so they stay "not found" (WS-006).
SELECT sk.* FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
WHERE sk.id = $1 AND sk.deleted_at IS NULL;

-- name: SoftDeleteSkill :one
-- WS-005/CORE-007: soft delete now, background purge after the 30-day grace
-- period. Version rows stay (0005 trigger); the search document is removed by
-- the caller in the same transaction.
UPDATE skills SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSkillSummary :exec
-- Mutable skill metadata only; versions themselves are immutable (iron rule 4).
-- Workspace scoped like every other statement here, even though the only caller
-- already re-read the skill through GetSkill: an unscoped UPDATE sitting in the
-- query file is a cross-tenant write waiting for its second caller.
UPDATE skills SET summary = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2;

-- name: ListSkills :many
SELECT * FROM skills
WHERE workspace_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountSkillVersions :one
SELECT count(*) FROM skill_versions
WHERE skill_id = $1;

-- name: GetSkillSource :one
-- Import provenance for one version (DISC-003: source URL, ref/commit, fetch
-- time, content hash). Workspace scoped like every other read in this file even
-- though the only caller reaches it through an already-scoped version row: an
-- unscoped read sitting in the query file is a cross-tenant read waiting for its
-- second caller.
SELECT * FROM skill_sources
WHERE id = $1 AND workspace_id = $2;

-- name: GetSkillEnrichment :one
-- The index-time model output for one skill (ADR-013 §1), so the detail view can
-- show the enriched summary and task examples *labelled as model-generated*
-- rather than as the package's own words. `summary` comes back too because it is
-- the frontmatter description and the fallback when enrichment is still pending.
SELECT summary, enriched_summary, task_examples, tags,
       enrichment_status, enrichment_model, enrichment_prompt_version
FROM search_documents
WHERE skill_id = $1 AND workspace_id = $2;
