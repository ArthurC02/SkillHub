ALTER TABLE runs ADD COLUMN origin_key text
    CONSTRAINT runs_origin_key_shape CHECK (origin_key IS NULL OR (btrim(origin_key) <> '' AND length(origin_key) <= 200));

CREATE UNIQUE INDEX runs_workspace_origin_key ON runs (workspace_id, origin_key) WHERE origin_key IS NOT NULL;

COMMENT ON COLUMN runs.origin_key IS
    'The caller''s name for the request that started this run; asking again with the same name returns this run instead of starting another.';

ALTER TABLE run_continuations ADD COLUMN answered_as_simulated_user boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN run_continuations.answered_as_simulated_user IS
    'The answers were given by a test case''s simulated user, not by a person.';
