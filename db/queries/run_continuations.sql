-- name: InsertRunContinuation :one
INSERT INTO run_continuations (run_id, workspace_id, continues_run_id, questions, answers, answered_by)
VALUES (@run_id, @workspace_id, @continues_run_id, @questions, @answers, @answered_by)
RETURNING *;

-- name: RunWasContinued :one
SELECT EXISTS (
    SELECT 1 FROM run_continuations WHERE continues_run_id = @run_id AND workspace_id = @workspace_id
) AS continued;

-- name: CountContinuationChain :one
WITH RECURSIVE chain AS (
    SELECT c.continues_run_id, 1 AS depth
    FROM run_continuations c
    WHERE c.run_id = @run_id AND c.workspace_id = @workspace_id
    UNION ALL
    SELECT c.continues_run_id, chain.depth + 1
    FROM run_continuations c
    JOIN chain ON c.run_id = chain.continues_run_id
    WHERE c.workspace_id = @workspace_id
)
SELECT COALESCE(max(depth), 0)::integer AS depth FROM chain;

-- name: GetRunSettings :one
SELECT * FROM run_settings;

-- name: SetRunSettings :one
INSERT INTO run_settings (singleton, continuation_rounds, reason, set_by)
VALUES (true, @continuation_rounds, @reason, @set_by)
ON CONFLICT (singleton) DO UPDATE
SET continuation_rounds = EXCLUDED.continuation_rounds,
    reason = EXCLUDED.reason,
    set_by = EXCLUDED.set_by,
    set_at = now()
RETURNING *;
