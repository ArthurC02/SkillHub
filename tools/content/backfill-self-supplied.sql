-- 0036 / ADR-045: mark the skills a workspace supplied itself, so their owners
-- can download them.
--
-- Not in the migration, deliberately. The fact lives in three tables and the
-- statement is a policy decision — which rows stop being blocked — so it is run
-- knowingly and prints what it touched, rather than executing inside a schema
-- change nobody reads.
--
-- Run once per deployment, after 0036:
--   psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/backfill-self-supplied.sql
--
-- Idempotent: re-running matches nothing, because the predicate requires
-- 'unknown'.

BEGIN;

-- The four conditions, and none of them is redundant:
--
--   redistribution = 'unknown'    only the default is rewritten. An 'allowed' a
--                                 curator established and a 'blocked' verdict
--                                 are both decisions; this must not overwrite
--                                 either.
--   NOT w.is_catalog              the catalogue is loaded through the same
--                                 upload endpoint (tools/content/import_seed.py),
--                                 so without this every curated skill would be
--                                 marked self-supplied and every fork of one
--                                 released. This condition is the whole safety
--                                 of the change.
--   forked_from_skill_id IS NULL  a fork supplied nothing; it received a copy,
--                                 and its verdict travels from the source.
--   EXISTS(skill_sources)         there is an actual import record. A skills row
--                                 with no source row is not something this can
--                                 claim a workspace brought in.
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

-- What is still blocked afterwards, so the operator sees the remainder rather
-- than assuming the backfill covered everything.
SELECT sk.redistribution, w.is_catalog, count(*) AS skills
  FROM skills sk
  JOIN workspaces w ON w.id = sk.workspace_id
 WHERE sk.deleted_at IS NULL
 GROUP BY 1, 2
 ORDER BY 1, 2;

COMMIT;
