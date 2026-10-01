-- name: LockQueueSchema :exec
SELECT pg_advisory_lock(hashtextextended('skillhub:queue-schema', 0));
