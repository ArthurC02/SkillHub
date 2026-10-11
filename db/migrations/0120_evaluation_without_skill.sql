ALTER TABLE evaluation_batches ADD COLUMN with_baseline boolean NOT NULL DEFAULT false;

ALTER TABLE evaluation_batch_items ADD COLUMN skill_withheld boolean NOT NULL DEFAULT false;

ALTER TABLE evaluation_batch_items DROP CONSTRAINT evaluation_batch_items_pkey;
ALTER TABLE evaluation_batch_items ADD PRIMARY KEY (batch_id, test_case_id, trial, skill_withheld);

COMMENT ON COLUMN evaluation_batches.with_baseline IS
    'Every trial is also run once with the skill withheld, so the report can show what the skill adds.';
COMMENT ON COLUMN evaluation_batch_items.skill_withheld IS
    'This trial ran the same test case without loading the skill.';
