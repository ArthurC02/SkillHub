-- name: LockQueueSchema :exec
SELECT pg_advisory_lock(hashtextextended('skillhub:queue-schema', 0));

-- name: UnlockQueueSchema :exec
SELECT pg_advisory_unlock(hashtextextended('skillhub:queue-schema', 0));
