-- 0010_public_catalog: an explicit public scope for anonymous search (CORE-006).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Before this column, public search (DISC-001) queried search_documents with no
-- tenant predicate at all, so every user's private forks were readable by an
-- anonymous caller — the leak WS-002 ("使用者只能存取自己的私有內容") and iron
-- rule 3 forbid. Publicness now has to be granted, never inherited: the default
-- is false, so a workspace created by signup is private forever unless an
-- operator flips this flag on the curation workspace.
--
-- ponytail: workspace-level granularity. Everything in a catalog workspace is
-- public; upgrade path is a per-skill skills.visibility column, needed once the
-- CONTENT-003 promotion workflow wants drafts to sit beside published entries
-- in the same workspace.
ALTER TABLE workspaces ADD COLUMN is_catalog boolean NOT NULL DEFAULT false;

CREATE INDEX workspaces_is_catalog_idx ON workspaces (id) WHERE is_catalog;
