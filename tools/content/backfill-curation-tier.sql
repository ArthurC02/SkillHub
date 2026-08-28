-- Record the PDM-002 curation verdict for the fifteen entries that passed it.
--
-- Mechanism: skills.curation_tier + skills.curated_version_id (migration 0042).
-- Decision and evidence: tools/content/seed-skills.json (`tier` plus the nine
-- `checks`, all nine `pass` for these fifteen since 2026-08-28) and
-- docs/plans/mvp/content/curated-skill-list.md §2, whose per-check judgements
-- these names come from.
--
-- Data, not schema, so it is not in the migration — a fresh database has no
-- skills to curate, and a migration that wrote a review verdict about rows it
-- cannot see would be a fixture pretending to be a review. Re-runnable.
--
-- Identification is by name inside the catalog workspace. That is the same
-- weakness restrict-anthropic-sa-display.sql calls out and for the same reason:
-- the seed import recorded every source as `upload` with no URL, so there is no
-- provenance column to match on. The names below are the fifteen `tier:
-- "curated"` entries of seed-skills.json; if that file changes, this list has to
-- change with it, and nothing enforces that. It is a list of fifteen names in
-- one place rather than a rule, because curation *is* a list of names — the
-- moment it becomes derivable it has stopped being a human review.
--
-- Forks are deliberately not included, which is the opposite of what the
-- restriction backfill does. A licence hold is a fact about the source and stays
-- true in a copy; a curation verdict says a person read the bytes in the
-- catalogue, and nobody read the fork. See 0042.
WITH curated(name) AS (
    VALUES ('excel-insert'), ('excel-freeze'), ('handoff'), ('excel-format'),
           ('brand-guidelines'), ('internal-comms'), ('humanizer'),
           ('line-edit'), ('ai-written-check'), ('data-analyst'),
           ('data-cleanliness-scan'), ('csv-to-json'), ('text-to-numeric'),
           ('excel-deduplicate'), ('excel-find-duplicates')
),
-- The version the review examined. The reviews in curated-skill-list.md were
-- carried out against the seed import, and no catalogue skill has had a second
-- version since, so the newest version *is* the reviewed one here. That
-- coincidence is why this can be a backfill at all; it will not hold for the
-- next review, which is why 0042 stores the version rather than deriving it.
target AS (
    SELECT sk.id AS skill_id,
           (SELECT v.id FROM skill_versions v
             WHERE v.skill_id = sk.id
             ORDER BY v.version_number DESC LIMIT 1) AS version_id
    FROM skills sk
    JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
    JOIN curated c ON c.name = sk.name
    WHERE sk.deleted_at IS NULL
)
UPDATE skills sk
SET curation_tier = 'curated',
    curated_version_id = t.version_id,
    updated_at = now()
FROM target t
WHERE sk.id = t.skill_id
  -- A skill with no version cannot be curated: the verdict is about bytes.
  AND t.version_id IS NOT NULL
  AND (sk.curation_tier, sk.curated_version_id) IS DISTINCT FROM ('curated', t.version_id);

-- Expected: 15 catalog rows, 0 forks. Verify with
--   SELECT w.is_catalog, sk.name, sk.curation_tier,
--          sk.curated_version_id = (SELECT v.id FROM skill_versions v
--                                    WHERE v.skill_id = sk.id
--                                    ORDER BY v.version_number DESC LIMIT 1) AS is_newest
--   FROM skills sk JOIN workspaces w ON w.id = sk.workspace_id
--   WHERE sk.curation_tier = 'curated' ORDER BY w.is_catalog DESC, sk.name;
