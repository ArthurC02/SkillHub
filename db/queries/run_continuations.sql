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

-- name: ListEarlierQuestions :many
WITH RECURSIVE chain AS (
    SELECT c.continues_run_id, c.questions, 1 AS depth
    FROM run_continuations c
    WHERE c.run_id = @run_id AND c.workspace_id = @workspace_id
    UNION ALL
    SELECT c.continues_run_id, c.questions, chain.depth + 1
    FROM run_continuations c
    JOIN chain ON c.run_id = chain.continues_run_id
    WHERE c.workspace_id = @workspace_id
)
SELECT questions FROM chain ORDER BY depth DESC;

-- name: ListContinuationChainEnds :many
WITH RECURSIVE chain AS (
    SELECT r.id AS start_id, r.id AS run_id, 0 AS depth
    FROM runs r
    WHERE r.workspace_id = @workspace_id AND r.id = ANY(sqlc.arg(run_ids)::uuid[])
    UNION ALL
    SELECT chain.start_id, c.run_id, chain.depth + 1
    FROM run_continuations c
    JOIN chain ON c.continues_run_id = chain.run_id
    WHERE c.workspace_id = @workspace_id
)
SELECT DISTINCT ON (chain.start_id)
    chain.start_id, start_run.test_case_snapshot_id AS start_snapshot_id,
    r.id AS end_id, r.status AS end_status
FROM chain
JOIN runs r ON r.id = chain.run_id AND r.workspace_id = @workspace_id
JOIN runs start_run ON start_run.id = chain.start_id AND start_run.workspace_id = @workspace_id
ORDER BY chain.start_id, chain.depth DESC;

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
