-- name: CreateEvaluation :one
INSERT INTO evaluations (
    workspace_id, run_id, status, overall, evidence_complete,
    judge_model, judge_prompt_version, rubric_version
)
VALUES (
    @workspace_id, @run_id, 'pending', 'undetermined', false,
    @judge_model, @judge_prompt_version, @rubric_version
)
RETURNING *;

-- name: SupersedeCurrentEvaluation :execrows
-- Run before CreateEvaluation in the same transaction: evaluations_current_key is a
-- non-deferrable partial unique index, so inserting first would collide.
UPDATE evaluations SET superseded_at = now(), updated_at = now()
WHERE run_id = @run_id AND workspace_id = @workspace_id AND superseded_at IS NULL;

-- name: CompleteEvaluation :one
UPDATE evaluations SET
    status = 'completed',
    overall = @overall,
    summary = @summary,
    criterion_results = @criterion_results,
    deterministic_findings = @deterministic_findings,
    judge_model = @judge_model,
    judge_prompt_version = @judge_prompt_version,
    rubric_version = @rubric_version,
    evidence_complete = @evidence_complete,
    cost_usd = @cost_usd,
    cost_source = @cost_source,
    evaluated_at = now(),
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND status = 'pending'
RETURNING *;

-- name: FailEvaluation :one
UPDATE evaluations SET
    status = 'failed',
    summary = @summary,
    deterministic_findings = @deterministic_findings,
    evidence_complete = @evidence_complete,
    evaluated_at = now(),
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND status = 'pending'
RETURNING *;

-- name: GetCurrentEvaluation :one
SELECT * FROM evaluations
WHERE run_id = $1 AND workspace_id = $2 AND superseded_at IS NULL;

-- name: RecordEvaluationModelUsage :exec
INSERT INTO evaluation_model_usage (
    evaluation_id, workspace_id, operation, model, prompt_version,
    prompt_tokens, completion_tokens, cost_usd, cost_source
)
SELECT @evaluation_id, @workspace_id, @operation, @model, @prompt_version, @prompt_tokens,
       @completion_tokens, @cost_usd, @cost_source
FROM evaluations
WHERE id = @evaluation_id AND workspace_id = @workspace_id
ON CONFLICT (evaluation_id, operation) DO NOTHING;

-- name: GetEvaluationRevision :one
SELECT * FROM evaluations
WHERE id = $1 AND run_id = $2 AND workspace_id = $3;

-- name: GetEvaluation :one
SELECT * FROM evaluations
WHERE id = $1 AND workspace_id = $2;

-- name: ListEvaluationRevisions :many
SELECT * FROM evaluations
WHERE run_id = $1 AND workspace_id = $2
ORDER BY created_at DESC, id DESC;

-- name: SetEvaluationFeedback :one
UPDATE evaluations SET
    feedback_helpful = @feedback_helpful,
    feedback_comment = @feedback_comment,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CreateEvaluationSuggestion :one
INSERT INTO evaluation_suggestions (
    workspace_id, evaluation_id, category, problem, evidence,
    target_path, proposed_content, expected_impact
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListEvaluationSuggestions :many
SELECT * FROM evaluation_suggestions
WHERE evaluation_id = $1 AND workspace_id = $2
ORDER BY created_at, id;

-- name: GetEvaluationSuggestion :one
SELECT * FROM evaluation_suggestions
WHERE id = $1 AND workspace_id = $2;

-- name: DecideSuggestion :one
UPDATE evaluation_suggestions SET
    decision = @decision,
    decided_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: MarkSuggestionsApplied :execrows
UPDATE evaluation_suggestions SET
    applied_skill_version_id = @skill_version_id
WHERE id = ANY(@ids::uuid[])
  AND workspace_id = @workspace_id
  AND decision = 'accepted';

-- name: FindLiveTraceEvents :many
SELECT event_id FROM trace_events
WHERE workspace_id = @workspace_id AND run_id = @run_id
  AND event_id = ANY(@event_ids::uuid[]);

-- name: ListStalePendingEvaluations :many
SELECT id, workspace_id, run_id
FROM evaluations
WHERE status = 'pending'
  AND superseded_at IS NULL
  AND created_at < @stale_before
ORDER BY created_at
LIMIT @result_limit;

-- name: ListCurrentEvaluations :many
SELECT run_id, status, overall
FROM evaluations
WHERE workspace_id = $1 AND run_id = ANY(sqlc.arg(run_ids)::uuid[])
  AND superseded_at IS NULL;
