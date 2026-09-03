-- 0053_skill_category: persist the PDM-001 category so the catalogue can be
-- browsed by what a skill is for, not only by how much review it has had.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- 01 §8 / PDM-001 chose three categories on 2026-08-14 — documents, writing,
-- data — and every one of the 45 seed entries carries one in
-- tools/content/seed-skills.json. None of that reached a user: the import
-- path takes a package and nothing else, so `?category=` answered 400 and the
-- only trace of the taxonomy on screen was a disabled <select> inside a
-- collapsed panel (05 R-19; 04 丙-134's neighbour).
--
-- Nullable, on purpose. This is 05 R-19's option (a): the value is a curation
-- judgement, written by the backfill for catalogue rows a person classified
-- and by nobody else. A user-imported skill has no category until the owner
-- decides how it gets one (R-19 (b), model-classified at index time, is the
-- recorded upgrade path). The read path renders NULL as 尚未定值 — the
-- platform has not decided — never as a guessed shelf (02:DISC-004,
-- 設計 §2.9), and a NOT NULL DEFAULT would be exactly that guess.
--
-- On `skills`, like curation_tier (0042), and unlike curation_tier it IS
-- copied onto a fork: a category says what the bytes are for, and a fork is
-- the same bytes in another workspace. A review verdict is about who read
-- them; a category is about what they do.
ALTER TABLE skills
    ADD COLUMN category text
        CHECK (category IS NULL OR category IN ('documents', 'writing', 'data'));

COMMENT ON COLUMN skills.category IS
    'PDM-001 category: documents | writing | data, or NULL when the platform has not assigned one (05 R-19). Copied onto forks. See 0053.';
