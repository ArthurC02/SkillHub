-- name: CreateSkillSource :one
-- The three generator columns are NULL for git and upload; 0037's one-way CHECK
-- requires all three when source_type is 'generated' (GEN-005).
INSERT INTO skill_sources (
    workspace_id, source_type, source_url, source_ref, content_hash, fetched_at,
    task_description, generator_model, generator_prompt_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSkillByName :one
SELECT * FROM skills
WHERE workspace_id = $1 AND name = $2 AND deleted_at IS NULL;

-- name: GetVersionBySkillAndHash :one
-- Duplicate-content detection (SKILL-001, INGEST-005): same content on the
-- same skill returns the existing immutable version instead of a new row.
SELECT * FROM skill_versions
WHERE skill_id = $1 AND content_hash = $2;

-- name: CountGeneratedSkills :one
-- The generation allowance's counter (GEN-004, ADR-047 決策 5).
--
-- It counts the rows generation produced, the same way PDM-010 counts the runs
-- themselves rather than keeping a balance column (ADR-028 決策 2) — so there is
-- nothing to decrement and nothing that can decrement wrongly. Two of ADR-047
-- 決策 2's rules fall out of that for free: a retry writes no second row, so the
-- unit is one generation and not one gateway call; and a generation that failed
-- validation writes no row at all, so it costs nothing.
--
-- There is a third case, and it is only safe because of something elsewhere:
-- duplicate content returns from persistVersion BEFORE CreateSkillSource, so a
-- paid generation could write no row at all. It cannot happen today because
-- importZip refuses a generated package whose name collides with an existing
-- skill, which means generation always creates a fresh skills row and a fresh
-- row has no version to duplicate. Remove that guard and this counter starts
-- undercharging silently.
--
-- `oldest` is when the window frees up again, matching CountQuotaRuns' shape.
SELECT
    count(*)::bigint AS used,
    min(fetched_at)::timestamptz AS oldest
FROM skill_sources
WHERE workspace_id = @workspace_id
  AND source_type = 'generated'
  AND fetched_at > @since;
