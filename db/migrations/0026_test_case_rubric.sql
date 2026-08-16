-- 0026_test_case_rubric: where CONTENT-007's rubric is edited, and where it is frozen.
-- Shape and reasoning: docs/plans/mvp/content/writing-rubrics.md §2 and §6.2 G1.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- CONTENT-007 clause 3 asks for a *user-editable* rubric that the judge answers
-- item by item with quoted evidence. Two of those three words were missing from
-- the data model: 0004 gave `test_cases` and `test_case_snapshots` nothing but
-- `acceptance_criteria`, so a rubric had nowhere to be stored and nothing to be
-- frozen into. `evaluations.rubric_version` (0024) has existed since M3 batch 1
-- and has had no source to read from.
--
-- Not a table of its own, for the reason evaluation-design §6.4 gives: a rubric
-- is a strengthening of the acceptance criteria, not a second mechanism. Its
-- items map onto the same `criterion_results` entries the criteria produce, so a
-- table here would add a join and a second representation of one list. Same
-- judgement 0004 made for `acceptance_criteria` itself.
--
-- Nullable, and deliberately not defaulted to '{}': "this test case has no
-- rubric" and "this test case has an empty rubric" are different statements, and
-- only the first one is true of the 45 baseline test cases.

-- (a) the editable draft ------------------------------------------------------
ALTER TABLE test_cases
    -- {version, items: [{id, text, weight?, evidence_required}]} - the shape of
    -- llm-internal.yaml's Rubric plus the version string, which lives here
    -- because `evaluations.rubric_version` has to be able to say which rubric a
    -- verdict was reached under. The judge request carries only `items`: that
    -- schema is additionalProperties:false and has no version field.
    ADD COLUMN rubric jsonb,

    -- The floor under the Go validation, not a replacement for it. Go checks the
    -- things that need context (an item id must name a criterion of this same
    -- test case, lengths, item count); what the database can settle on its own is
    -- that a stored rubric is not a shape nothing can read back.
    ADD CONSTRAINT test_cases_rubric_shape CHECK (
        rubric IS NULL OR (
            jsonb_typeof(rubric -> 'version') = 'string'
            AND jsonb_typeof(rubric -> 'items') = 'array'
            AND NOT jsonb_path_exists(
                rubric,
                '$.items[*] ? (!exists(@.id) || !exists(@.text) || !exists(@.evidence_required))')
        )
    );

-- (b) the frozen copy a run executed ------------------------------------------
-- Iron rule 4, and the same sentence the TestCases screen already puts on the
-- criteria: starting a run freezes the rubric as it stands, so editing it
-- afterwards changes the *next* run and nothing about one that already happened.
-- Without this column an "editable rubric" would silently rewrite the standard a
-- past verdict was reached against, which is the exact failure ADR-003 exists to
-- prevent. The 0005 trigger freezes it along with everything else on the row.
ALTER TABLE test_case_snapshots
    ADD COLUMN rubric jsonb,
    ADD CONSTRAINT test_case_snapshots_rubric_shape CHECK (
        rubric IS NULL OR (
            jsonb_typeof(rubric -> 'version') = 'string'
            AND jsonb_typeof(rubric -> 'items') = 'array'
            AND NOT jsonb_path_exists(
                rubric,
                '$.items[*] ? (!exists(@.id) || !exists(@.text) || !exists(@.evidence_required))')
        )
    );
