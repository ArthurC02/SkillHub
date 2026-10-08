ALTER TABLE credit_entries
ADD COLUMN request_fingerprint bytea
CHECK (request_fingerprint IS NULL OR length(request_fingerprint) = 32);
