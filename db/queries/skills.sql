-- Every read here is workspace scoped (iron rule 3). The caller resolves workspace_id
-- from the session, never from request input.

-- name: CreateSkill :one
-- access_restriction is a parameter rather than a default because a fork has to
-- carry the licensing hold of what it was forked from (0023): a fork is another
-- copy of the same materials, so the hold travels with it the way the license
-- expression and its provenance tier already do. Import passes NULL.
--
-- redistribution travels the same way and for the same reason (0027, ADR-027
-- decision 4): a fork is another copy of the same licensed material, so a fork
-- that dropped the verdict would be the way around it. Nullable in the argument
-- only, so import can say "I have no verdict to pass on" and get the column's
-- own conservative default instead of having to name it — 'unknown' blocks, and
-- the caller that would have to spell it out is the one least placed to judge.
INSERT INTO skills (workspace_id, name, summary, forked_from_skill_id, forked_from_version_id,
                    access_restriction, redistribution)
VALUES ($1, $2, $3, $4, $5, $6, coalesce(sqlc.narg('redistribution')::text, 'unknown'))
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
-- The owner's own skills, newest first, with the one measurement fact the list
-- can answer for itself (04 丙-31).
--
-- `verified_at` is the newest version's creation time, the same definition the
-- public search path uses. `verified_source_id` is what keeps that definition
-- honest here and there is no equivalent need in search: **a fork gets a version
-- row of its own whose created_at is the moment somebody pressed Fork**, and
-- nothing was scanned at that moment — the bytes were scanned in the workspace
-- it was forked from. Import sets source_id, fork leaves it NULL (see
-- registry.Fork), so a NULL here means "this version arrived as a copy" and the
-- caller reports 未測量 rather than a timestamp that reads like a fresh scan.
-- Catalogue search never sees that case: it only lists catalog workspaces, and
-- nothing forks into one.
SELECT sqlc.embed(sk), ver.created_at AS verified_at, ver.source_id AS verified_source_id
FROM skills sk
LEFT JOIN LATERAL (
    SELECT v.created_at, v.source_id
    FROM skill_versions v
    WHERE v.skill_id = sk.id
    ORDER BY v.version_number DESC
    LIMIT 1
) ver ON true
WHERE sk.workspace_id = $1 AND sk.deleted_at IS NULL
ORDER BY sk.created_at DESC
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
SELECT summary, enriched_summary, task_examples, tags, limitations,
       enrichment_status, enrichment_model, enrichment_prompt_version
FROM search_documents
WHERE skill_id = $1 AND workspace_id = $2;
