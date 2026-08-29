-- Closed-beta queries: the PDM-010 run allowance (ADR-028), the O11Y-004 funnel
-- events and the BETA-003/004/005 feedback channel (ADR-029, 0029).

-- name: CountQuotaRuns :one
-- The PDM-010 allowance, computed from the runs themselves. There is no balance
-- column and there is not going to be one (ADR-028 決策 2): a balance is a cache
-- that drifts, and a refund would be a write that a crash, a restart or a failed
-- cleanup can miss. Here a refund is a predicate — the failure classes below are
-- excluded from the count, so a run refunded by policy was never counted, and a
-- path nobody wrote still gets the refund right.
--
-- Counted from the moment a run entered `preparing` (PDM-010 計數方式): that is
-- when resources are really taken. A failure during `provisioning` therefore never
-- counted in the first place, which is why "自動退還" only ever has to cover
-- platform-side failures after that point.
--
-- Excluded classes are exactly PDM-010's refund list, mapped onto 0018's fixed
-- vocabulary:
--   provider_error      the provider could not carry it
--   platform_error      the control plane's own fault
--   capability_mismatch gate B's baseline check refused it
-- Deliberately still counted: workload_error (the skill failed at its own job),
-- cancelled (the user asked), and timeout (the run had its full time and spent
-- it). All three are valid product use, and PDM-010 lists the first two by name
-- under 不退還.
--
-- One rolling boundary per call, computed by the caller: the daily limit and the
-- window limit are the same question over two spans, so they are the same query
-- run twice rather than two queries that could drift apart in what they count.
--
-- `oldest` is what /me/quota reports as the reset moment: the window is rolling,
-- so there is no monthly reset instant to name — the next allowance appears when
-- the oldest counted run falls out the back of the window.
--
-- count(DISTINCT) because a run reaching `preparing` twice would double count. The
-- state machine has no path back (internal/run/state.go), so this is belt and
-- braces — but the allowance is the platform's only real cost ceiling, and the
-- cheap guard belongs on the side that cannot overcharge silently.
SELECT
    count(DISTINCT r.id)::bigint AS used,
    min(t.occurred_at)::timestamptz AS oldest
FROM runs r
JOIN run_status_transitions t ON t.run_id = r.id AND t.to_status = 'preparing'
WHERE r.workspace_id = @workspace_id
  AND t.occurred_at > @since
  AND (r.failure_class IS NULL OR r.failure_class NOT IN (
          'provider_error', 'platform_error', 'capability_mismatch'));

-- name: GetWorkspaceCreatedAt :one
-- When this workspace came into existence, which is what decides whether it is
-- still inside PDM-010's first 30-day window (20 runs) or past it (30 a month).
-- No owner argument, unlike GetWorkspace: the caller already resolved the
-- workspace from the session, and the quota check runs inside the create-run
-- transaction where a second ownership test would only re-answer a question that
-- transaction has already answered.
SELECT created_at FROM workspaces WHERE id = $1;

-- name: InsertAnalyticsEvent :exec
-- One funnel event (O11Y-004). Every column here is declared in 0029 and there is
-- no free-text one among them: the whitelist is the schema, so an attribute the
-- schema never declared cannot be written even by mistake (ADR-029 決策 2).
--
-- Fire and forget by design. The caller must never fail a user's request because
-- this insert failed — analytics is explicitly not a source of truth (ADR-029
-- 決策 1), so losing a row costs a data point and nothing else. That is also why
-- it is not in the caller's transaction: making a search fail because a
-- measurement failed would invert the priority.
INSERT INTO analytics_events (
    event_name, session_id, workspace_id,
    query_length, query_language, result_count, has_results, filters_applied,
    -- `arrival` and `arrival_rank` left this list with 0040 (04 丙-59). They were
    -- written on every skill_detail_viewed row and always said the same thing,
    -- because the front end never sent the parameters they were derived from —
    -- a dimension that distinguished nothing while looking alive.
    skill_id,
    artifact_id, target
) VALUES (
    @event_name, @session_id, @workspace_id,
    @query_length, @query_language, @result_count, @has_results, @filters_applied,
    @skill_id,
    @artifact_id, @target
);

-- name: DetachWorkspaceAnalytics :execrows
-- Account deletion de-identifies rather than deletes (ADR-029 決策 5): the events
-- stay so quarter-over-quarter funnel comparisons survive, the link to the person
-- does not. The purge anonymises the workspace row rather than deleting it, so the
-- ON DELETE SET NULL in 0029 never fires and this is what actually does the work.
UPDATE analytics_events SET workspace_id = NULL WHERE workspace_id = $1;

-- name: DetachWorkspaceFeedback :execrows
-- Same treatment for the qualitative feedback (ADR-029 決策 5, beta-design §6.2):
-- what a beta tester said about the product is the evidence a scope review rests
-- on, and deleting it because they closed their account would destroy the review
-- rather than protect them. The words stay, their name does not.
UPDATE feedback_reports SET workspace_id = NULL, user_id = NULL
WHERE workspace_id = $1;

-- name: InsertFeedbackReport :exec
-- BETA-003/004/005. Workspace and user come from the session (iron rule 3); run_id
-- has already been checked to belong to that workspace by the caller, or dropped.
INSERT INTO feedback_reports (workspace_id, user_id, kind, message, page_path, run_id)
VALUES (@workspace_id, @user_id, @kind, @message, @page_path, @run_id);

-- name: RunInWorkspace :one
-- Does this run belong to the caller's workspace? Used only to decide whether a
-- feedback report may carry the run_id it quoted. Existence is private (WS-006),
-- and here a false answer drops the field rather than refusing the report.
SELECT EXISTS (SELECT 1 FROM runs WHERE id = $1 AND workspace_id = $2);

-- name: GetIdentityProviderIDs :many
-- The provider identities behind one account (ADR-020: identity and account are
-- separate, so a user can have more than one). BETA-001's admission list is keyed
-- by provider_user_id, matching the SEC-011 precedent of a roster that lives in
-- deployment configuration rather than in a role table.
SELECT provider, provider_user_id FROM user_identities WHERE user_id = $1;
-- name: DeleteExpiredFeedbackReports :execrows
-- FEEDBACK_RETENTION, and it is the fourth time this repository has found the
-- same shape: a data class being collected with nothing that ever removes it.
-- 0029 gave feedback_reports a table, a 2000-character message column and a
-- de-identification statement (DetachWorkspaceFeedback above) — and no DELETE, no
-- sweep, and no line in GET /policy/data-retention, whose text meanwhile declares
-- there is no free-text column anywhere. That sentence is true of
-- analytics_events and false of this deployment.
--
-- Deleting rather than de-identifying, and the two are not in tension.
-- DetachWorkspaceFeedback answers "this person closed their account" and keeps
-- the words, because ADR-029 決策 5 rests a scope review on them. This answers
-- "the words are older than the window anyone agreed to keep them for", which no
-- amount of de-identification satisfies: a 2000-character free-text field a
-- participant wrote about where they got stuck is exactly the kind of content
-- NFR-002 says must have a ratified deadline before it is collected at all.
--
-- The cutoff is the caller's, like every other retention sweep here: the window
-- is deployment configuration, fail-closed on an unset FEEDBACK_RETENTION
-- (cmd/maintenance purge-feedback), because a default here would be this process
-- enforcing a retention nobody signed, by deleting.
--
-- No immutability trigger on this table (0029 gave it none), so no purge flag.
DELETE FROM feedback_reports WHERE created_at < $1;
