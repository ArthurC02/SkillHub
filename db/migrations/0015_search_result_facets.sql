-- 0015_search_result_facets: the per-result columns DISC-002 requires on a
-- search row, plus the enrichment's 限制 field for DISC-003 一般模式.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Why these live in the projection at all. The detail view (DISC-006) answers
-- risk by re-scanning the stored package on every read, which keeps
-- skillpkg.Validate the single definition of what the platform discloses. A
-- search page cannot do that: it would be one object-store fetch plus one zip
-- scan per hit, up to 100 of them, inside the NFR-004 latency budget. So the
-- scan result is projected here instead — and search_documents is the right
-- place for it precisely because it is a rebuildable projection and never a
-- source of truth (ADR-010): a drifted value is repaired by reindex, not by a
-- migration.
--
-- The other four DISC-002 columns need no storage and get none:
--   * 來源層級 (tier) is `indexed` for everything that reached this table.
--   * 相容狀態 spec_validation is `passed` for any skill with a saved version —
--     a blocked report is never persisted, so being listed is the evidence.
--   * 最近驗證時間 is the latest skill_versions.created_at, one lateral join
--     away and immutable, so it cannot drift from the content it describes.
--   * 依賴 is the dependency bucket of `tags` below.

ALTER TABLE search_documents
    -- tags was a space-joined string of every bucket, on the reasoning that
    -- nothing read the buckets apart. DISC-002 (依賴 per result) and DISC-003
    -- (輸入/輸出/依賴 as separate answers) both now do, and a flat list cannot
    -- say which token was which. Dropping rather than adding a second column:
    -- this is a projection, so the old values are not data being lost — the
    -- next import or `reindex` refills them, and until then a row reads as
    -- having no tags, which is the same thing a pending enrichment reads as.
    --
    -- Nullable, like `scan` below: NULL means no enrichment has written tags
    -- for this row, which is a different statement from '{}' ("the model looked
    -- and found no inputs, outputs, tools or dependencies"). The pending path
    -- has to be able to say the first one.
    DROP COLUMN tags,
    ADD COLUMN tags jsonb,
    -- What the document states the Skill does not do or needs (DISC-003 限制),
    -- newline-joined, same convention as task_examples. Model-written, so it is
    -- displayed with the rest of the enrichment block and labelled as such
    -- (ADR-013).
    ADD COLUMN limitations text NOT NULL DEFAULT '',
    -- Static scan summary as of the last time this row was written:
    --   {"warnings": <int>, "codes": ["script-file", ...]}
    -- NULL means no scan has been projected for this row, which is reported as
    -- unknown and never as a clean scan (DISC-004 不得自行推定為通過). Rows
    -- written by the SQL-only ReindexAll phase keep whatever they had, and rows
    -- created by it start NULL, which is the honest answer for both.
    --
    -- Error-level findings are not counted: any one of them blocks the import,
    -- so a package that reached this table has none.
    ADD COLUMN scan jsonb;
