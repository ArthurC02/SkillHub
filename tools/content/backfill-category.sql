-- Record the PDM-001 category for the 45 catalogue entries that have one.
--
-- Mechanism: skills.category (migration 0053). Decision and evidence:
-- tools/content/seed-skills.json `skills[].category` (documents 10 / writing 10
-- / data 25), the boundary rule that file states — create/format = documents,
-- tidy/filter/dedupe/merge/split/replace = data — and
-- docs/plans/mvp/content/curated-skill-list.md.
--
-- Data, not schema, for backfill-curation-tier.sql's reason: a fresh database
-- has no skills to classify. Re-runnable.
--
-- Identification is by name inside the catalog workspace, with the same known
-- weakness the curation backfill records: the seed import stored no source
-- URL, so a name is the only handle. The 45 names are unique in the seed file
-- today; if that stops being true this script has to change with it.
--
-- Forks ARE included, unlike the curation backfill: 0053 defines category as a
-- fact about the bytes, so a copy of the bytes has the same category. A fork
-- made before this script ran gets its value here; a fork made after gets it
-- from CreateSkill.

WITH named(name) AS (
    VALUES ('excel-insert'), ('excel-freeze'), ('handoff'), ('excel-format'),
           ('document-format-skills'), ('course-quiz-builder'), ('docx'), ('pdf'),
           ('pptx'), ('xlsx')
)
UPDATE skills sk
SET category = 'documents', updated_at = now()
FROM named n
WHERE sk.name = n.name
  AND sk.deleted_at IS NULL
  AND (
    EXISTS (SELECT 1 FROM workspaces w WHERE w.id = sk.workspace_id AND w.is_catalog)
    OR sk.forked_from_skill_id IN (
        SELECT c.id FROM skills c
        JOIN workspaces w ON w.id = c.workspace_id AND w.is_catalog
        WHERE c.name = n.name)
  )
  AND sk.category IS DISTINCT FROM 'documents';

WITH named(name) AS (
    VALUES ('brand-guidelines'), ('internal-comms'), ('humanizer'), ('line-edit'),
           ('ai-written-check'), ('cringe-check'), ('full-review'), ('copyright-creative-work'),
           ('sokrati'), ('shorten')
)
UPDATE skills sk
SET category = 'writing', updated_at = now()
FROM named n
WHERE sk.name = n.name
  AND sk.deleted_at IS NULL
  AND (
    EXISTS (SELECT 1 FROM workspaces w WHERE w.id = sk.workspace_id AND w.is_catalog)
    OR sk.forked_from_skill_id IN (
        SELECT c.id FROM skills c
        JOIN workspaces w ON w.id = c.workspace_id AND w.is_catalog
        WHERE c.name = n.name)
  )
  AND sk.category IS DISTINCT FROM 'writing';

WITH named(name) AS (
    VALUES ('data-analyst'), ('data-cleanliness-scan'), ('csv-to-json'), ('text-to-numeric'),
           ('excel-deduplicate'), ('excel-find-duplicates'), ('excel-filter'), ('excel-validate'),
           ('excel-merge'), ('excel-split'), ('excel-sort'), ('excel-regex-clean'),
           ('excel-scout'), ('excel-delete'), ('excel-mapping-replace'), ('excel-date-to-text'),
           ('standardise-country-names'), ('unicode-consistency'), ('date-wrangling'), ('json-restructure'),
           ('data-shape'), ('data-comparability'), ('add-data-dictionary'), ('pii-flag'),
           ('add-iso3166')
)
UPDATE skills sk
SET category = 'data', updated_at = now()
FROM named n
WHERE sk.name = n.name
  AND sk.deleted_at IS NULL
  AND (
    EXISTS (SELECT 1 FROM workspaces w WHERE w.id = sk.workspace_id AND w.is_catalog)
    OR sk.forked_from_skill_id IN (
        SELECT c.id FROM skills c
        JOIN workspaces w ON w.id = c.workspace_id AND w.is_catalog
        WHERE c.name = n.name)
  )
  AND sk.category IS DISTINCT FROM 'data';

-- Expected: 45 catalog rows (documents 10 / writing 10 / data 25) plus any
-- forks of them. Verify with
--   SELECT category, count(*) FROM skills sk
--   JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
--   WHERE sk.deleted_at IS NULL GROUP BY 1 ORDER BY 1;
