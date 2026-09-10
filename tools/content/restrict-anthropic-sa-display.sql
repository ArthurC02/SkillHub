WITH RECURSIVE restricted(name) AS (
    VALUES ('docx'), ('pdf'), ('pptx'), ('xlsx')
),
roots AS (
    SELECT sk.id
    FROM skills sk
    JOIN workspaces w ON w.id = sk.workspace_id AND w.is_catalog
    JOIN restricted r ON r.name = sk.name
    WHERE sk.deleted_at IS NULL
),
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

