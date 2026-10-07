-- name: RecordDatabaseSize :one
INSERT INTO database_size_samples (sampled_on, database_bytes)
VALUES (@sampled_on, pg_database_size(current_database()))
ON CONFLICT (sampled_on) DO UPDATE
SET database_bytes = EXCLUDED.database_bytes, sampled_at = now()
RETURNING database_bytes;

-- name: ListDatabaseSizesSince :many
SELECT sampled_on, database_bytes
FROM database_size_samples
WHERE sampled_on >= @since
ORDER BY sampled_on;
