-- 0061_skill_category_source: record who assigned a skill's PDM-001 category,
-- so the read side can tell a curation judgement from an owner's own answer
-- (05 R-19 item 4). Applied migrations are immutable: fix forward with a new
-- file, never edit this one.
--
-- 0053 gave `skills.category` a value and exactly one writer: the curation
-- backfill. Every user-imported skill stayed NULL, so `?category=` matched
-- none of them and the taxonomy served only the 45 seeded rows (05 R-19's
-- background). R-19 does not close that gap by having a model classify the
-- skill at index time -- 02:DISC-004 and 設計 §2.9 forbid a guessed shelf, and
-- the model that would have to guess is the enrich-skill prompt pinned to the
-- F1 0.955 and poisoning measurements in `05` R-53: changing its prompt to add
-- a classification question means paying to re-run both of those, not writing
-- a line of code. The chosen answer is PUT /skills/{id}/category: the owner
-- says what their own bytes are for.
--
-- That makes two writers of `category`, and a reader who cannot tell them
-- apart would read an owner's own guess as a platform verdict, or vice versa
-- -- exactly the confusion NFR-001 exists to prevent between axes, now showing
-- up *inside* one axis instead of between two.
--
-- 'curated' and 'owner', not 'model': there is no third writer yet, and adding
-- a value nothing can produce would be exactly the kind of unenforced promise
-- `04` 丙-63 keeps finding in this codebase (a grace-period sentence with no
-- job behind it, a purge deadline nobody signed). The day something classifies
-- a skill by running a model over it, that day's migration adds 'model' to the
-- CHECK next to the query that actually writes it.
--
-- The pairing CHECK -- both NULL or both set -- exists because `category`
-- without a `category_source` is unattributable: a reader who sees `documents`
-- with no source cannot tell a curation judgement from an owner's own claim,
-- which is exactly the confusion this migration exists to end.
ALTER TABLE skills
    ADD COLUMN category_source text
        CHECK (category_source IS NULL OR category_source IN ('curated', 'owner')),
    ADD CONSTRAINT skills_category_and_source_together
        CHECK ((category IS NULL) = (category_source IS NULL));

-- Backfill: every row 0053's curation backfill gave a category is a curation
-- judgement, because it is the only writer that has ever run before this
-- migration.
UPDATE skills SET category_source = 'curated' WHERE category IS NOT NULL;

COMMENT ON COLUMN skills.category_source IS
    'Who assigned skills.category: curated (PDM-001 backfill) | owner (PUT /skills/{id}/category, 05 R-19). NULL iff category is NULL. See 0061.';
