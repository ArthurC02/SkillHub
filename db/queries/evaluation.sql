-- Evaluation (EVAL-001, ADR-025, ADR-026). Every statement is workspace scoped
-- (iron rule 3), including the ones the worker runs: an evaluation job payload is
-- not a wider authority than a request.
--
-- Nothing here writes `runs`. `runs.status` answers "what happened when this
-- executed", `evaluations.overall` answers "was the task achieved" (ADR-025), and
-- an UPDATE on runs from this file would be the regression that ADR forbids.

-- name: CreateEvaluation :one
-- The pending row, written before the judge is called so that an evaluation which
-- dies mid-flight is a visible `pending`/`failed` state rather than a row that
-- never existed (design §4.3). overall starts at `undetermined` because that is
-- what a verdict that has not been reached actually is, and evidence_complete
-- starts false because nothing has been read yet - both are corrected by
-- CompleteEvaluation.
INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete)
VALUES ($1, $2, 'pending', 'undetermined', false)
RETURNING *;

-- name: SupersedeCurrentEvaluation :execrows
-- ADR-026 decision 1: re-evaluation is append-only and exactly one row per run has
-- superseded_at IS NULL.
--
-- Run in the SAME transaction as CreateEvaluation and BEFORE it. The partial
-- unique index evaluations_current_key is not deferrable, so an insert that runs
-- first collides with the standing verdict instead of replacing it - the order
-- here is the mechanism, not a preference.
UPDATE evaluations SET superseded_at = now(), updated_at = now()
WHERE run_id = @run_id AND workspace_id = @workspace_id AND superseded_at IS NULL;

-- name: CompleteEvaluation :one
-- The verdict. After this the 0024 trigger freezes the row apart from feedback and
-- superseded_at, so everything the report will ever say is written here at once.
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
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'completed'
RETURNING *;

-- name: FailEvaluation :one
-- The evaluation could not produce a verdict (judge unreachable, evidence
-- unreadable). It stays `undetermined` and keeps whatever deterministic findings
-- were already established: those came from the platform's own records and are
-- still true even when the model leg failed. `status <> 'completed'` refuses to
-- turn a verdict somebody has read into a failure.
UPDATE evaluations SET
    status = 'failed',
    summary = @summary,
    deterministic_findings = @deterministic_findings,
    evidence_complete = @evidence_complete,
    evaluated_at = now(),
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND status <> 'completed'
RETURNING *;

-- name: GetCurrentEvaluation :one
-- The standing verdict: GET /runs/{id}/evaluation without ?revision=. Served by
-- evaluations_current_key, which also guarantees this cannot match twice.
SELECT * FROM evaluations
WHERE run_id = $1 AND workspace_id = $2 AND superseded_at IS NULL;

-- name: GetEvaluationRevision :one
-- One particular judgement by id. run_id is in the predicate as well as the id:
-- a revision id from another run must not resolve through this run's URL.
SELECT * FROM evaluations
WHERE id = $1 AND run_id = $2 AND workspace_id = $3;

-- name: GetEvaluation :one
-- One evaluation by id, current or superseded. Used by the suggestion surface,
-- which reaches an evaluation through a suggestion or a request body rather than
-- through a run's URL - workspace scope is what makes that safe (iron rule 3).
SELECT * FROM evaluations
WHERE id = $1 AND workspace_id = $2;

-- name: ListEvaluationRevisions :many
-- Newest first (GET /runs/{id}/evaluation/revisions). Uses evaluations_run_id_idx.
SELECT * FROM evaluations
WHERE run_id = $1 AND workspace_id = $2
ORDER BY created_at DESC, id DESC;

-- name: SetEvaluationFeedback :one
-- EVAL-001 clause 4. One of the three columns 0024's trigger leaves writable on a
-- completed row: the user is allowed to change their mind, so this replaces the
-- previous answer rather than appending a second one.
UPDATE evaluations SET
    feedback_helpful = @feedback_helpful,
    feedback_comment = @feedback_comment,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CreateEvaluationSuggestion :one
-- EVAL-002 clause 1. Written by the evaluation job once the verdict is stored:
-- a suggestion hangs off the evaluation that produced it, so re-evaluating under a
-- new rubric produces its own set instead of appending to somebody else's.
-- `decision` is left at its default `pending` - proposing is not deciding.
INSERT INTO evaluation_suggestions (
    workspace_id, evaluation_id, category, problem, evidence,
    target_path, proposed_content, expected_impact
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListEvaluationSuggestions :many
-- GET /runs/{id}/suggestions, after the caller's current evaluation has been
-- resolved. Oldest first, which is the order they were proposed in.
SELECT * FROM evaluation_suggestions
WHERE evaluation_id = $1 AND workspace_id = $2
ORDER BY created_at, id;

-- name: GetEvaluationSuggestion :one
-- One suggestion in the caller's workspace. Existence is private (WS-006), so a
-- row belonging to somebody else is the same answer as no row.
SELECT * FROM evaluation_suggestions
WHERE id = $1 AND workspace_id = $2;

-- name: DecideSuggestion :one
-- EVAL-002 clause 3. Repeatable: a later call replaces the decision, so this is an
-- UPDATE and not an append. `decided_at` moves with it because 0024 refuses a
-- decision with no timestamp - the two are one fact.
--
-- It applies nothing. Turning accepted suggestions into a version is
-- POST /skills/{id}/versions/from-suggestions, and keeping the two apart is what
-- makes five accepted suggestions one new version rather than five.
UPDATE evaluation_suggestions SET
    decision = @decision,
    decided_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: MarkSuggestionsApplied :execrows
-- EVAL-002 clause 4: the suggestion now points at the **new** version. The
-- `decision = 'accepted'` predicate repeats 0024's CHECK rather than trusting the
-- caller to have filtered - nothing gets applied that was not accepted, and the
-- statement that enforces it is the one doing the write.
UPDATE evaluation_suggestions SET
    applied_skill_version_id = @skill_version_id
WHERE id = ANY(@ids::uuid[])
  AND workspace_id = @workspace_id
  AND decision = 'accepted';

-- name: FindLiveTraceEvents :many
-- ADR-026 decision 2: an evidence reference outlives the evidence, so `available`
-- is answered at read time rather than trusted from write time - trace_events is
-- dropped by partition and a ref stored as available would silently keep claiming
-- the original is still there. One query per report, bounded by the refs it cites.
SELECT event_id FROM trace_events
WHERE workspace_id = @workspace_id AND run_id = @run_id
  AND event_id = ANY(@event_ids::uuid[]);
