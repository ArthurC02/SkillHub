-- name: LockTestSchema :exec
SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0));

-- name: UnlockTestSchema :exec
SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0));
