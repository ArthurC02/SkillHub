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

