ALTER TABLE test_cases ADD COLUMN simulated_user_facts text[] NOT NULL DEFAULT '{}'
    CONSTRAINT test_cases_simulated_user_facts_count CHECK (cardinality(simulated_user_facts) <= 10);

ALTER TABLE test_case_snapshots ADD COLUMN simulated_user_facts text[] NOT NULL DEFAULT '{}';

ALTER TABLE evaluation_batches ADD COLUMN settled_at timestamptz;
UPDATE evaluation_batches SET settled_at = created_at WHERE status = 'launched';
ALTER TABLE evaluation_batches ADD CONSTRAINT evaluation_batches_settled_once_launched
    CHECK (settled_at IS NULL OR status = 'launched');

DROP INDEX evaluation_batches_launching_idx;
CREATE INDEX evaluation_batches_unsettled_idx ON evaluation_batches (workspace_id)
    WHERE status <> 'stopped' AND settled_at IS NULL;

COMMENT ON COLUMN test_cases.simulated_user_facts IS
    'What the person in this test case knows but did not say; a batch answers the skill''s questions with them.';
COMMENT ON COLUMN test_case_snapshots.simulated_user_facts IS
    'The simulated person''s facts, frozen with the rest of the test case.';
COMMENT ON COLUMN evaluation_batches.settled_at IS
    'Every trial has ended and no simulated answer is still owed.';
