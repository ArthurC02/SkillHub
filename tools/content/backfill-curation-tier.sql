WITH curated(name) AS (
    VALUES ('excel-insert'), ('excel-freeze'), ('handoff'), ('excel-format'),
           ('brand-guidelines'), ('internal-comms'), ('humanizer'),
           ('line-edit'), ('ai-written-check'), ('data-analyst'),
           ('data-cleanliness-scan'), ('csv-to-json'), ('text-to-numeric'),
           ('excel-deduplicate'), ('excel-find-duplicates')
),
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
  AND t.version_id IS NOT NULL
  AND (sk.curation_tier, sk.curated_version_id) IS DISTINCT FROM ('curated', t.version_id);

