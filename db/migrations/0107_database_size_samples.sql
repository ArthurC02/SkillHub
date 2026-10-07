CREATE TABLE database_size_samples (
    sampled_on     date PRIMARY KEY,
    database_bytes bigint NOT NULL CHECK (database_bytes >= 0),
    sampled_at     timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE ON database_size_samples TO skillhub_purge;
