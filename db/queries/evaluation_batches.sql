-- name: InsertEvaluationBatch :one
INSERT INTO evaluation_batches (workspace_id, skill_id, skill_version_id, trials, with_baseline, created_by)
VALUES (@workspace_id, @skill_id, @skill_version_id, @trials, @with_baseline, @created_by)
RETURNING *;

-- name: InsertEvaluationBatchTrials :exec
INSERT INTO evaluation_batch_items (batch_id, workspace_id, test_case_id, position, trial, summary_hash, skill_withheld)
SELECT @batch_id, @workspace_id, @test_case_id, @position, trial, @summary_hash, withheld
FROM generate_series(1, sqlc.arg(trials)::integer) AS trial,
     unnest(CASE WHEN sqlc.arg(with_baseline)::boolean THEN ARRAY[false, true] ELSE ARRAY[false] END) AS withheld;

-- name: GetEvaluationBatch :one
SELECT * FROM evaluation_batches WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListEvaluationBatches :many
SELECT * FROM evaluation_batches
WHERE workspace_id = @workspace_id AND skill_id = @skill_id
ORDER BY created_at DESC
LIMIT 50;

-- name: ListEvaluationBatchItems :many
SELECT * FROM evaluation_batch_items
WHERE batch_id = @batch_id AND workspace_id = @workspace_id
ORDER BY skill_withheld, position, trial;

-- name: ClaimNextBatchItem :one
SELECT i.batch_id, i.test_case_id, i.trial, i.skill_withheld, i.summary_hash, b.skill_id, b.skill_version_id, b.created_by
FROM evaluation_batch_items i
JOIN evaluation_batches b ON b.id = i.batch_id AND b.workspace_id = i.workspace_id
WHERE b.workspace_id = @workspace_id AND b.status = 'launching' AND i.run_id IS NULL
ORDER BY b.created_at, i.trial, i.position, i.skill_withheld
LIMIT 1;

-- name: LinkBatchItemRun :execrows
UPDATE evaluation_batch_items SET run_id = @run_id
WHERE batch_id = @batch_id AND test_case_id = @test_case_id AND trial = @trial
  AND skill_withheld = @skill_withheld AND workspace_id = @workspace_id AND run_id IS NULL;

-- name: StopEvaluationBatch :execrows
UPDATE evaluation_batches SET status = 'stopped', stop_reason = @stop_reason
WHERE id = @id AND workspace_id = @workspace_id AND status = 'launching';

-- name: MarkBatchesLaunched :execrows
UPDATE evaluation_batches b SET status = 'launched'
WHERE b.workspace_id = @workspace_id AND b.status = 'launching'
  AND NOT EXISTS (
      SELECT 1 FROM evaluation_batch_items i
      WHERE i.batch_id = b.id AND i.workspace_id = b.workspace_id AND i.run_id IS NULL
  );

-- name: HasUnsettledBatch :one
SELECT EXISTS (
    SELECT 1 FROM evaluation_batches
    WHERE workspace_id = @workspace_id AND status <> 'stopped' AND settled_at IS NULL
) AS unsettled;

-- name: ListWorkspacesWithUnsettledBatches :many
SELECT DISTINCT workspace_id FROM evaluation_batches WHERE status <> 'stopped' AND settled_at IS NULL;

-- name: ListUnsettledBatchTrials :many
SELECT b.id AS batch_id, b.status, b.created_by, i.run_id
FROM evaluation_batches b
JOIN evaluation_batch_items i ON i.batch_id = b.id AND i.workspace_id = b.workspace_id
WHERE b.workspace_id = @workspace_id AND b.status <> 'stopped' AND b.settled_at IS NULL
ORDER BY b.created_at, b.id;

-- name: MarkBatchSettled :execrows
UPDATE evaluation_batches SET settled_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND status = 'launched' AND settled_at IS NULL;
