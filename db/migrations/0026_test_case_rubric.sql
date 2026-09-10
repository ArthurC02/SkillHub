ALTER TABLE test_cases
    ADD COLUMN rubric jsonb,

    ADD CONSTRAINT test_cases_rubric_shape CHECK (
        rubric IS NULL OR (
            jsonb_typeof(rubric -> 'version') = 'string'
            AND jsonb_typeof(rubric -> 'items') = 'array'
            AND NOT jsonb_path_exists(
                rubric,
                '$.items[*] ? (!exists(@.id) || !exists(@.text) || !exists(@.evidence_required))')
        )
    );

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
