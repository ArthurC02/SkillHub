WITH RECURSIVE
osi_redistributable(expression) AS (
    VALUES ('0BSD'),
           ('Apache-2.0'),
           ('BSD-2-Clause'),
           ('BSD-3-Clause'),
           ('CC0-1.0'),
           ('ISC'),
           ('MIT'),
           ('MPL-2.0'),
           ('Unlicense')
),
source_available(pattern) AS (
    VALUES ('LicenseRef-%Source-Available%'),
           ('Proprietary%')
),
latest AS (
    SELECT DISTINCT ON (sv.skill_id)
           sv.skill_id,
           btrim(COALESCE(sv.license_expression, '')) AS expression
    FROM skill_versions sv
    ORDER BY sv.skill_id, sv.version_number DESC
),
roots AS (
    SELECT sk.id,
           CASE
               WHEN l.expression IS NULL OR l.expression IN ('', 'NOASSERTION', 'NONE')
                   THEN 'unknown'
               WHEN EXISTS (
                   SELECT 1 FROM source_available sa WHERE l.expression ILIKE sa.pattern
               ) THEN 'blocked'
               WHEN EXISTS (
                   SELECT 1 FROM osi_redistributable o WHERE o.expression = l.expression
               ) THEN 'allowed'
               ELSE 'unknown'
           END AS verdict
    FROM skills sk
    LEFT JOIN latest l ON l.skill_id = sk.id
    WHERE sk.deleted_at IS NULL
      AND sk.forked_from_skill_id IS NULL
),
lineage(id, verdict) AS (
    SELECT id, verdict FROM roots
    UNION
    SELECT sk.id, lg.verdict
    FROM skills sk
    JOIN lineage lg ON sk.forked_from_skill_id = lg.id
    WHERE sk.deleted_at IS NULL
)
UPDATE skills sk
SET redistribution = lg.verdict,
    updated_at     = now()
FROM lineage lg
WHERE sk.id = lg.id
  AND sk.redistribution IS DISTINCT FROM lg.verdict;

SELECT w.is_catalog,
       sk.redistribution,
       count(*)
FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id
WHERE sk.deleted_at IS NULL
GROUP BY 1, 2
ORDER BY 1 DESC, 2;

SELECT sk.name,
       w.is_catalog,
       COALESCE(lv.license_expression, '(none)') AS license_expression,
       COALESCE(lv.license_source, '(none)')     AS license_source
FROM skills sk
JOIN workspaces w ON w.id = sk.workspace_id
LEFT JOIN LATERAL (
    SELECT sv.license_expression, sv.license_source
    FROM skill_versions sv
    WHERE sv.skill_id = sk.id
    ORDER BY sv.version_number DESC
    LIMIT 1
) lv ON true
WHERE sk.deleted_at IS NULL
  AND sk.redistribution = 'unknown'
ORDER BY w.is_catalog DESC, sk.name;
