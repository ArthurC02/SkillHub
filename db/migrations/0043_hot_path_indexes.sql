-- Four partial indexes for four foreign keys that account deletion and the skill
-- purges walk on every pass, and that nothing indexed.
--
-- Every one of them is partial on `IS NOT NULL`, and that is not a micro
-- optimisation: after a purge the column IS null on exactly the rows that were
-- swept, so the sparse half is the whole working set and the dense half never
-- needs finding. Same reasoning the existing partial indexes are built on
-- (0018's runs_active_idx, 0035's outbox_events_unpublished_idx): the predicate
-- is copied from the statement, not invented for the index.
--
-- Nothing here changes behaviour. They are in one migration because they share
-- one reason — every statement below is on the deletion path a participant is
-- promised inside 30 days, and a compliance deadline resting on a sequential
-- scan of a table designed to grow without bound is a coupling that only
-- announces itself once the table is big, which is after the deadline matters.

-- Serves: beta.sql DetachWorkspaceAnalytics
--   UPDATE analytics_events SET workspace_id = NULL WHERE workspace_id = $1;
--
-- analytics_events is RANGE partitioned on occurred_at (0029) and workspace_id is
-- not the partition key, so this statement scans every partition, every time.
-- It is also the only table in the schema that grows on every search.
--
-- CREATE INDEX on a partitioned parent is a partitioned index: Postgres creates a
-- matching child index on every existing partition and on every partition
-- attached later, so partition.MaintainMonthly's future months inherit it with no
-- change to that package. Two restrictions come with that and both are fine here:
-- CONCURRENTLY is not allowed on a partitioned table (this takes a brief lock on
-- a table whose rows are append-only telemetry), and the parent index is created
-- INVALID until every child is built, which happens inside this statement.
CREATE INDEX analytics_events_workspace_id_idx ON analytics_events (workspace_id)
    WHERE workspace_id IS NOT NULL;

-- Serves: beta.sql DetachWorkspaceFeedback
--   UPDATE feedback_reports SET workspace_id = NULL, user_id = NULL
--   WHERE workspace_id = $1;
--
-- The table's only index is (kind, created_at) (0029), which cannot serve this.
CREATE INDEX feedback_reports_workspace_id_idx ON feedback_reports (workspace_id)
    WHERE workspace_id IS NOT NULL;

-- Serves: the referential integrity check behind every
--   DELETE FROM skill_versions ...
-- in governance.sql (PurgeUnreferencedSkills, PurgeSkillsPastDeletionGrace).
--
-- 0042 made skills.curated_version_id ON DELETE SET NULL and argued the choice
-- carefully, but a SET NULL still has to find the referencing rows, and without
-- an index that is a sequential scan of `skills` per deleted version. Sparse by
-- construction: only a curated skill has one.
CREATE INDEX skills_curated_version_id_idx ON skills (curated_version_id)
    WHERE curated_version_id IS NOT NULL;

-- Serves: the same two DELETEs, plus the `purgeable` exclusion added on
-- 2026-08-29 that reads this column directly:
--   NOT EXISTS (SELECT 1 FROM skills f
--               JOIN skill_versions v ON v.id = f.forked_from_version_id
--               WHERE v.skill_id = sk.id)
--
-- NO ACTION rather than SET NULL (0003), so the check is "is there any
-- referencing row", which an index answers without reading the table.
CREATE INDEX skills_forked_from_version_id_idx ON skills (forked_from_version_id)
    WHERE forked_from_version_id IS NOT NULL;
