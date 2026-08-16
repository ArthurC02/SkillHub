-- Apply the owner's 方案 C decision of 2026-08-16 to the four `anthropics/skills`
-- source-available Skills: hold the materials, keep the listing.
--
-- Decision and its reasoning: docs/plans/mvp/governance/anthropic-sa-license-memo.md §3 方案 C
-- (＋ its 2026-08-16 追記). Mechanism: skills.access_restriction (migration 0023).
--
-- Data, not schema, so it is not in the migration — a fresh database has no
-- skills to restrict, and a migration that wrote a licensing decision about rows
-- it cannot see would be a fixture pretending to be a decision. Re-runnable.
--
-- Identification is by name inside the catalog workspace, which is weaker than
-- it looks and is called out rather than hidden: the seed import recorded every
-- source as `upload` with no URL (skill_sources.source_url is empty for all 45),
-- so there is no provenance column to match `anthropics/skills` on. The mapping
-- name → source lives in tools/content/seed-skills.json (`source_id:
-- anthropic-sa`), and these four are the whole of that set. If the catalogue ever
-- carries a second skill named `pdf` from a different source, this file has to
-- become a lookup by content hash first.
--
-- Run with:
--   psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/restrict-anthropic-sa-display.sql

WITH RECURSIVE restricted(name) AS (
    VALUES ('docx'), ('pdf'), ('pptx'), ('xlsx')
),
-- The catalogue rows the decision names.
roots AS (
    SELECT sk.id
    FROM skills sk
    JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
    JOIN restricted r ON r.name = sk.name
    WHERE sk.deleted_at IS NULL
),
-- Plus every fork already made from them. New forks inherit the hold at fork
-- time (registry.Fork), but the M2 baseline forked these four before the column
-- existed, and those copies are exactly what the hold is about. Recursive so a
-- fork of a fork is reached too.
lineage AS (
    SELECT id FROM roots
    UNION
    SELECT sk.id
    FROM skills sk
    JOIN lineage l ON sk.forked_from_skill_id = l.id
    WHERE sk.deleted_at IS NULL
)
UPDATE skills
SET access_restriction = 'license-review',
    updated_at = now()
WHERE id IN (SELECT id FROM lineage)
  AND access_restriction IS DISTINCT FROM 'license-review';

-- Expected: 4 catalog rows plus the baseline's forks of them. Verify with
--   SELECT w.is_catalog, sk.name, sk.access_restriction
--   FROM skills sk JOIN workspaces w ON w.id = sk.workspace_id
--   WHERE sk.access_restriction IS NOT NULL ORDER BY w.is_catalog DESC, sk.name;
