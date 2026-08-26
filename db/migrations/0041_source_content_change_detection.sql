-- 0041_source_content_change_detection: the column that lets a re-fetch report an
-- EDGE instead of shouting the same thing every sweep. Applied migrations are
-- immutable: fix forward.
--
-- `02:SEC-003` and `02` CONTENT-009 both say upstream change is detected 「以重抓
-- 並與保存的內容雜湊比對進行」, and `02:SEC-007` 第 2 條 delegates to the same
-- sentence. CheckSources sent one HEAD, which answers "is this URL still there",
-- not "is it still the same thing". So deletion was caught and REWRITE and
-- LICENCE CHANGE were not -- and those last two are what SEC-007 exists for.
--
-- The comparison value was already here: skill_sources.content_hash, written on
-- every import and read by nothing. What was missing is somewhere to record that
-- the comparison has already failed once.
--
-- WHY A COLUMN AND NOT JUST THE COMPARISON
--
-- content_hash must not be overwritten when the upstream changes. It is the hash
-- of the snapshot this workspace actually holds, and iron rule 4 makes that
-- snapshot immutable -- rewriting it would make the row describe bytes nobody
-- has. But then the comparison fails again on every subsequent sweep, and an
-- audit row per sweep buries the one that matters under thousands of repeats.
-- That is the same argument unavailable_since already settled for availability,
-- and this is its sibling: set on the first sweep that sees a different hash,
-- and it stays set, because "the upstream stopped matching what we hold" does
-- not become false again by itself. Only a re-import can clear it, and a
-- re-import writes a new row.
--
-- Deliberately NOT nullable-with-a-boolean-twin and NOT a status enum: the
-- question a human asks is "since when", and a timestamp answers both that and
-- "at all" with one column.
ALTER TABLE skill_sources ADD COLUMN content_changed_at timestamptz;

COMMENT ON COLUMN skill_sources.content_changed_at IS
    'First sweep on which a re-fetch hashed differently from content_hash. NULL means every check so far matched, or no check has compared content yet. Never cleared: the snapshot we hold does not become current again.';
