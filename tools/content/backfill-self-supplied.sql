BEGIN;

WITH marked AS (
    UPDATE skills sk
       SET redistribution = 'self_supplied', updated_at = now()
      FROM workspaces w
     WHERE w.id = sk.workspace_id
       AND sk.redistribution = 'unknown'
       AND sk.deleted_at IS NULL
       AND NOT w.is_catalog
       AND sk.forked_from_skill_id IS NULL
       AND EXISTS (
             SELECT 1 FROM skill_versions sv
              WHERE sv.skill_id = sk.id AND sv.source_id IS NOT NULL
           )
    RETURNING sk.id, sk.workspace_id
)
SELECT count(*) AS marked_self_supplied,
       count(DISTINCT workspace_id) AS workspaces_affected
  FROM marked;

SELECT sk.redistribution, w.is_catalog, count(*) AS skills
  FROM skills sk
  JOIN workspaces w ON w.id = sk.workspace_id
 WHERE sk.deleted_at IS NULL
 GROUP BY 1, 2
 ORDER BY 1, 2;

COMMIT;
