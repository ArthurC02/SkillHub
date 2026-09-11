-- name: CreateTestCase :one
INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt, acceptance_criteria)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTestCase :one
SELECT * FROM test_cases
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: LockTestCase :one
SELECT * FROM test_cases
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: ListTestCases :many
SELECT * FROM test_cases
WHERE workspace_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpdateTestCase :one
UPDATE test_cases SET name = $3, user_prompt = $4, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateTestCaseCriteria :one
UPDATE test_cases SET acceptance_criteria = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateTestCaseRubric :one
UPDATE test_cases SET rubric = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTestCase :one
UPDATE test_cases SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: CreateDataset :one
INSERT INTO datasets (
    workspace_id, test_case_id, file_name, content_type, size_bytes,
    content_hash, object_key, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateDatasetCleanupIntent :one
INSERT INTO dataset_object_cleanup_intents (workspace_id, object_key)
VALUES ($1, $2)
RETURNING *;

-- name: LockDatasetObjectKeySession :exec
SELECT pg_advisory_lock(hashtextextended('dataset-object:' || @object_key::text, 0));

-- name: UnlockDatasetObjectKeySession :one
SELECT pg_advisory_unlock(hashtextextended('dataset-object:' || @object_key::text, 0));

-- name: DeleteDatasetCleanupIntent :exec
DELETE FROM dataset_object_cleanup_intents
WHERE id = $1 AND workspace_id = $2;

-- name: GetDataset :one
SELECT * FROM datasets
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: ListDatasets :many
SELECT * FROM datasets
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
ORDER BY created_at;

-- name: SumDatasetUsage :one
SELECT count(*)::bigint AS file_count,
       coalesce(sum(size_bytes), 0)::bigint AS total_bytes
FROM datasets
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteDataset :one
UPDATE datasets SET deleted_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDatasetsByTestCase :many
UPDATE datasets SET deleted_at = now()
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: CreateTestCaseSnapshot :one
INSERT INTO test_case_snapshots (
    workspace_id, test_case_id, user_prompt, acceptance_criteria, dataset_refs, content_hash, rubric
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTestCaseSnapshot :one
SELECT * FROM test_case_snapshots
WHERE id = $1 AND workspace_id = $2;

-- name: ListTestCaseSnapshotIDs :many
SELECT id FROM test_case_snapshots
WHERE workspace_id = @workspace_id AND test_case_id = @test_case_id;

-- name: ListSnapshotTestCases :many
SELECT id, test_case_id FROM test_case_snapshots
WHERE workspace_id = @workspace_id AND id = ANY(@snapshot_ids::uuid[]);

-- name: SnapshotInputsStillAvailable :one
SELECT (
    tc.deleted_at IS NULL
    AND NOT EXISTS (
        SELECT 1 FROM jsonb_array_elements(s.dataset_refs) AS ref
        WHERE NOT EXISTS (
            SELECT 1 FROM datasets d
            WHERE d.id = (ref->>'dataset_id')::uuid
              AND d.workspace_id = s.workspace_id
              AND d.deleted_at IS NULL
              AND d.expires_at > now()
        )
    )
)::boolean AS available
FROM test_case_snapshots s
JOIN test_cases tc ON tc.id = s.test_case_id
WHERE s.id = @snapshot_id AND s.workspace_id = @workspace_id;

-- name: LockDatasetWorkspaceObjects :exec
SELECT pg_advisory_lock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: UnlockDatasetWorkspaceObjects :one
SELECT pg_advisory_unlock_shared(hashtextextended('workspace-objects:' || (sqlc.arg(workspace_id)::uuid)::text, 0));

-- name: ListSkillsWithTestCases :many
SELECT DISTINCT skill_id FROM test_cases WHERE skill_id = ANY(@skill_ids::uuid[]);
