-- name: CreateEvaluation :one
INSERT INTO evaluations (
    workspace_id, run_id, status, overall, evidence_complete,
    judge_model, judge_prompt_version, rubric_version
)
VALUES (
    @workspace_id, @run_id, @status, @overall, @evidence_complete,
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
    injection_check = sqlc.narg(injection_check),
    injection_probability = sqlc.narg(injection_probability),
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

-- name: LockCurrentEvaluation :one
SELECT * FROM evaluations
WHERE run_id = $1 AND workspace_id = $2 AND superseded_at IS NULL
FOR UPDATE;

-- name: LockEvaluation :one
SELECT * FROM evaluations
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

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

-- name: RecordSuggestionOutcome :exec
UPDATE evaluations SET suggestion_outcome = @suggestion_outcome
WHERE id = @id AND workspace_id = @workspace_id AND suggestion_outcome IS NULL;

-- name: SetEvaluationFeedback :one
UPDATE evaluations SET
    feedback_helpful = @feedback_helpful,
    feedback_comment = @feedback_comment,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ClearWorkspaceEvaluationComments :execrows
UPDATE evaluations SET feedback_comment = NULL, updated_at = now()
WHERE workspace_id = @workspace_id AND feedback_comment IS NOT NULL;

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

-- name: LockEvaluationSuggestion :one
SELECT * FROM evaluation_suggestions
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: DecideSuggestion :one
UPDATE evaluation_suggestions SET
    decision = @decision,
    decided_at = now(),
    decided_by = sqlc.narg(decided_by)
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: RecordSuggestionApplications :exec
INSERT INTO evaluation_suggestion_applications (workspace_id, suggestion_id, skill_version_id)
SELECT @workspace_id::uuid, suggestion_id, @skill_version_id::uuid
FROM unnest(@suggestion_ids::uuid[]) AS suggestion_id;

-- name: SetSuggestionsAppliedVersion :exec
UPDATE evaluation_suggestions SET applied_skill_version_id = @skill_version_id
WHERE id = ANY(@ids::uuid[]) AND workspace_id = @workspace_id;

-- name: ListEvaluationSuggestionApplications :many
SELECT a.suggestion_id, a.skill_version_id
FROM evaluation_suggestion_applications a
JOIN evaluation_suggestions s ON s.id = a.suggestion_id AND s.workspace_id = a.workspace_id
WHERE s.evaluation_id = @evaluation_id AND a.workspace_id = @workspace_id
ORDER BY a.suggestion_id, a.skill_version_id;

-- name: ListSuggestionsAppliedToVersion :many
SELECT s.evaluation_id, s.reflection_id, s.category, s.target_path
FROM evaluation_suggestion_applications a
JOIN evaluation_suggestions s ON s.id = a.suggestion_id AND s.workspace_id = a.workspace_id
WHERE a.skill_version_id = sqlc.arg(applied_skill_version_id) AND a.workspace_id = @workspace_id
ORDER BY s.created_at, s.id;

-- name: FindLiveTraceEvents :many
SELECT event_id FROM trace_events
WHERE workspace_id = @workspace_id AND run_id = @run_id
  AND event_id = ANY(@event_ids::uuid[]);

-- name: ListStalePendingEvaluations :many
SELECT id, workspace_id, run_id
FROM evaluations
WHERE status = ANY(@awaiting_statuses::text[])
  AND superseded_at IS NULL
  AND created_at < @stale_before
ORDER BY created_at
LIMIT @result_limit;

-- name: ListCurrentEvaluations :many
SELECT run_id, status, overall
FROM evaluations
WHERE workspace_id = $1 AND run_id = ANY(sqlc.arg(run_ids)::uuid[])
  AND superseded_at IS NULL;

-- name: ListEvaluationActivityFacts :many
SELECT run_id, status, overall, created_at, evaluated_at
FROM evaluations
WHERE workspace_id = @workspace_id AND superseded_at IS NULL
ORDER BY created_at DESC, id;

-- name: GetEvaluationSettings :one
SELECT * FROM evaluation_settings;

-- name: SetEvaluationSettings :one
INSERT INTO evaluation_settings (judge_panel, reason, set_by, set_at)
VALUES (@judge_panel, @reason, sqlc.narg(set_by), now())
ON CONFLICT (singleton) DO UPDATE
SET judge_panel = EXCLUDED.judge_panel, reason = EXCLUDED.reason, set_by = EXCLUDED.set_by, set_at = now()
RETURNING *;

-- name: SetEvaluationInjectionCheck :one
INSERT INTO evaluation_settings (judge_panel, injection_check, injection_check_credits_per_million_tokens,
                                 injection_check_reason, injection_check_set_at)
VALUES (false, @injection_check, @credits_per_million_tokens, @reason, now())
ON CONFLICT (singleton) DO UPDATE
SET injection_check = EXCLUDED.injection_check,
    injection_check_credits_per_million_tokens = EXCLUDED.injection_check_credits_per_million_tokens,
    injection_check_reason = EXCLUDED.injection_check_reason,
    injection_check_set_at = now()
RETURNING *;

-- name: SetEvaluationReflectionCandidates :one
INSERT INTO evaluation_settings (judge_panel, reflection_candidates, reflection_round_credit_cap,
                                 reflection_candidates_reason, reflection_candidates_set_at)
VALUES (false, @reflection_candidates, sqlc.narg(reflection_round_credit_cap), @reason, now())
ON CONFLICT (singleton) DO UPDATE
SET reflection_candidates = EXCLUDED.reflection_candidates,
    reflection_round_credit_cap = EXCLUDED.reflection_round_credit_cap,
    reflection_candidates_reason = EXCLUDED.reflection_candidates_reason,
    reflection_candidates_set_at = now()
RETURNING *;

-- name: ListRunsWithAnEvaluation :many
SELECT DISTINCT run_id FROM evaluations WHERE run_id = ANY(@run_ids::uuid[]);

-- name: CreateBatchReflection :one
INSERT INTO batch_reflections (
    workspace_id, batch_id, skill_version_id, dev_test_case_ids, outcome, runs_read, runs_not_read, created_by,
    candidate_focuses, candidate_outcomes
) VALUES (
    @workspace_id, @batch_id, @skill_version_id, @dev_test_case_ids::uuid[], @outcome, @runs_read, @runs_not_read, sqlc.narg(created_by),
    @candidate_focuses::text[], @candidate_outcomes::text[]
)
RETURNING *;

-- name: ListBatchReflections :many
SELECT * FROM batch_reflections
WHERE workspace_id = @workspace_id AND batch_id = @batch_id
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: GetBatchReflection :one
SELECT * FROM batch_reflections
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateReflectionSuggestion :one
INSERT INTO evaluation_suggestions (
    workspace_id, reflection_id, category, problem, evidence,
    target_path, proposed_content, expected_impact, candidate
) VALUES (
    @workspace_id, @reflection_id, @category, @problem, @evidence,
    @target_path, @proposed_content, @expected_impact, @candidate
)
RETURNING *;

-- name: ListReflectionSuggestions :many
SELECT * FROM evaluation_suggestions
WHERE reflection_id = ANY(@reflection_ids::uuid[]) AND workspace_id = @workspace_id
ORDER BY created_at, id;

-- name: ListReflectionSuggestionApplications :many
SELECT a.suggestion_id, a.skill_version_id
FROM evaluation_suggestion_applications a
JOIN evaluation_suggestions s ON s.id = a.suggestion_id AND s.workspace_id = a.workspace_id
WHERE s.reflection_id = @reflection_id AND a.workspace_id = @workspace_id
ORDER BY a.suggestion_id, a.skill_version_id;

-- name: InsertReflectionCandidateBatch :exec
INSERT INTO reflection_candidate_batches (
    workspace_id, reflection_id, candidate, skill_version_id, batch_id, skipped_suggestion_ids, created_by
) VALUES (
    @workspace_id, @reflection_id, @candidate, sqlc.narg(skill_version_id), sqlc.narg(batch_id),
    @skipped_suggestion_ids::uuid[], sqlc.narg(created_by)
)
ON CONFLICT (reflection_id, candidate) DO NOTHING;

-- name: ListReflectionCandidateBatches :many
SELECT * FROM reflection_candidate_batches
WHERE reflection_id = @reflection_id AND workspace_id = @workspace_id
ORDER BY candidate;

-- name: ListCandidateVersionsOfSkill :many
SELECT l.skill_version_id::uuid AS version_id, l.reflection_id, rounds.batch_id, rounds.round::integer AS round, l.candidate
FROM reflection_candidate_batches l
JOIN (
    SELECT br.id, br.batch_id, row_number() OVER (ORDER BY br.created_at, br.id) AS round
    FROM batch_reflections br
    WHERE br.workspace_id = @workspace_id AND br.skill_version_id = ANY(@version_ids::uuid[])
) rounds ON rounds.id = l.reflection_id
WHERE l.workspace_id = @workspace_id AND l.skill_version_id IS NOT NULL
ORDER BY rounds.round, l.candidate;

-- name: InsertReflectionAdoption :one
INSERT INTO reflection_adoptions (reflection_id, workspace_id, candidate, skill_version_id, adopted_by)
VALUES (@reflection_id, @workspace_id, @candidate, @skill_version_id, sqlc.narg(adopted_by))
ON CONFLICT (reflection_id) DO NOTHING
RETURNING *;

-- name: GetReflectionAdoption :one
SELECT * FROM reflection_adoptions WHERE reflection_id = @reflection_id AND workspace_id = @workspace_id;
