-- Test Lab: test cases, their acceptance criteria, datasets and run snapshots
-- (TEST-001/003/004/010). Every statement is workspace scoped (iron rule 3); the
-- caller resolves workspace_id from the session, never from request input.

-- name: CreateTestCase :one
INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt, acceptance_criteria)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTestCase :one
SELECT * FROM test_cases
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: LockTestCase :one
-- Same read as GetTestCase, but it serialises concurrent dataset uploads to one
-- test case. Without the lock two parallel uploads both measure the usage before
-- either has written its row and both pass a limit their sum breaks (TEST-004).
SELECT * FROM test_cases
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: ListTestCases :many
SELECT * FROM test_cases
WHERE workspace_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateTestCase :one
-- Drafts are editable; what a run actually used is frozen in test_case_snapshots
-- instead (ADR-003, iron rule 4), so editing here never rewrites history.
UPDATE test_cases SET name = $3, user_prompt = $4, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateTestCaseCriteria :one
UPDATE test_cases SET acceptance_criteria = $3, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateTestCaseRubric :one
-- CONTENT-007's editable rubric. Its own statement rather than a field on
-- UpdateTestCase for the same reason UpdateTestCaseCriteria is: the three are
-- edited from different screens and a shared statement would make each of them
-- able to blank the others by omission.
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

-- name: GetDataset :one
SELECT * FROM datasets
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: ListDatasets :many
SELECT * FROM datasets
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
ORDER BY created_at;

-- name: SumDatasetUsage :one
-- The live footprint of one test case, for the PDM-005 per-test-case caps
-- (≤ 20 files, ≤ 100 MB). Deleted rows do not count against the budget.
SELECT count(*)::bigint AS file_count,
       coalesce(sum(size_bytes), 0)::bigint AS total_bytes
FROM datasets
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteDataset :one
-- The row is marked deleted here; the object itself is removed by the caller
-- after the transaction commits (objstore.Remove is idempotent, iron rule 9).
UPDATE datasets SET deleted_at = now()
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDatasetsByTestCase :many
-- Deleting a test case takes its files with it (WS-002 刪除範圍). Returns the
-- object keys so the caller can remove them after the commit.
UPDATE datasets SET deleted_at = now()
WHERE test_case_id = $1 AND workspace_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: CreateTestCaseSnapshot :one
-- TEST-010: the frozen copy a run executes. Immutable once written (0005 trigger).
INSERT INTO test_case_snapshots (
    workspace_id, test_case_id, user_prompt, acceptance_criteria, dataset_refs, content_hash, rubric
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTestCaseSnapshot :one
SELECT * FROM test_case_snapshots
WHERE id = $1 AND workspace_id = $2;
