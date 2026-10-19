ALTER TABLE users ADD COLUMN erasure_expedited_at timestamptz;

ALTER TABLE users
    ADD CONSTRAINT users_erasure_expedited_needs_request
    CHECK (erasure_expedited_at IS NULL OR deletion_requested_at IS NOT NULL);
