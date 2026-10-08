-- name: RegisterPlatformAgent :exec
INSERT INTO platform_agents (name, purpose, model_role, daily_spend_cap_micros, tools, actions)
VALUES (@name, @purpose, @model_role, @daily_spend_cap_micros, @tools::text[], @actions::text[])
ON CONFLICT (name) DO UPDATE SET
    purpose = EXCLUDED.purpose,
    model_role = EXCLUDED.model_role,
    daily_spend_cap_micros = EXCLUDED.daily_spend_cap_micros,
    tools = EXCLUDED.tools,
    actions = EXCLUDED.actions;

-- name: ListPlatformAgents :many
SELECT * FROM platform_agents ORDER BY name;

-- name: SetPlatformAgentEnabled :one
UPDATE platform_agents SET enabled = @enabled, owner_id = @owner_id
WHERE name = @name
RETURNING *;

-- name: GetPlatformAgentBrake :one
SELECT * FROM platform_agent_brake;

-- name: EngagePlatformAgentBrake :one
INSERT INTO platform_agent_brake (engaged_by, reason)
VALUES (@engaged_by, @reason)
ON CONFLICT (engaged) DO UPDATE SET engaged_by = EXCLUDED.engaged_by, engaged_at = now(), reason = EXCLUDED.reason
RETURNING *;

-- name: ReleasePlatformAgentBrake :execrows
DELETE FROM platform_agent_brake;

-- name: StartPlatformAgentRun :one
INSERT INTO platform_agent_runs (agent_id)
SELECT a.id FROM platform_agents a
WHERE a.name = @name AND a.enabled AND NOT EXISTS (SELECT 1 FROM platform_agent_brake)
RETURNING id;

-- name: GetPlatformAgentRunGate :one
SELECT r.status, a.enabled, EXISTS (SELECT 1 FROM platform_agent_brake) AS braked
FROM platform_agent_runs r
JOIN platform_agents a ON a.id = r.agent_id
WHERE r.id = @id;

-- name: FinishPlatformAgentRun :execrows
UPDATE platform_agent_runs
SET status = @status, finished_at = now(), reason = sqlc.narg(reason), result = sqlc.narg(result)
WHERE id = @id AND status = 'running';

-- name: RecordPlatformAgentStep :exec
INSERT INTO platform_agent_steps (
    run_id, seq, tool, arguments, result, model, prompt_tokens, completion_tokens, usd_micros
) VALUES (
    @run_id, @seq, @tool, @arguments, @result, @model, @prompt_tokens, @completion_tokens, sqlc.narg(usd_micros)
);

-- name: PlatformAgentSpendSince :one
SELECT coalesce(sum(s.usd_micros), 0)::bigint
FROM platform_agent_steps s
JOIN platform_agent_runs r ON r.id = s.run_id
JOIN platform_agents a ON a.id = r.agent_id
WHERE a.name = @name AND s.created_at >= @since;

-- name: ListPlatformAgentRuns :many
SELECT r.id, a.name AS agent, r.status, r.reason, r.started_at, r.finished_at, r.result,
    count(s.seq)::integer AS steps,
    coalesce(sum(s.usd_micros), 0)::bigint AS usd_micros,
    count(s.seq) FILTER (WHERE s.usd_micros IS NULL)::integer AS unpriced_steps,
    max(s.created_at)::timestamptz AS last_step_at,
    count(*) OVER() AS total
FROM platform_agent_runs r
JOIN platform_agents a ON a.id = r.agent_id
LEFT JOIN platform_agent_steps s ON s.run_id = r.id
GROUP BY r.id, a.name
ORDER BY r.started_at DESC
LIMIT @row_limit;

-- name: GetPlatformAgentRun :one
SELECT r.id, a.name AS agent, r.status, r.reason, r.started_at, r.finished_at, r.result,
    count(s.seq)::integer AS steps,
    coalesce(sum(s.usd_micros), 0)::bigint AS usd_micros,
    count(s.seq) FILTER (WHERE s.usd_micros IS NULL)::integer AS unpriced_steps,
    max(s.created_at)::timestamptz AS last_step_at
FROM platform_agent_runs r
JOIN platform_agents a ON a.id = r.agent_id
LEFT JOIN platform_agent_steps s ON s.run_id = r.id
WHERE r.id = @id
GROUP BY r.id, a.name;

-- name: ListPlatformAgentSteps :many
SELECT seq, tool, arguments, result, model, prompt_tokens, completion_tokens, usd_micros, created_at
FROM platform_agent_steps
WHERE run_id = @run_id
ORDER BY seq;

-- name: ListMatchableFindings :many
SELECT id, status, cites, assignee_id, last_seen_at
FROM platform_agent_findings
WHERE agent_id = @agent_id AND (status = ANY (@live::text[]) OR last_seen_at >= @since);

-- name: OpenFinding :one
INSERT INTO platform_agent_findings (agent_id, title, cites)
VALUES (@agent_id, @title, @cites::text[])
RETURNING id;

-- name: SeeFinding :exec
UPDATE platform_agent_findings
SET title = @title, cites = @cites::text[], last_seen_at = now(), seen_count = seen_count + 1
WHERE id = @id;

-- name: SetFindingStatus :one
UPDATE platform_agent_findings
SET status = @status, assignee_id = coalesce(sqlc.narg(assignee_id), assignee_id), status_changed_at = now()
WHERE id = @id AND status = @from_status
RETURNING id;

-- name: GetFindingStatus :one
SELECT status FROM platform_agent_findings WHERE id = @id;

-- name: AppendFindingEvent :exec
INSERT INTO platform_agent_finding_events (finding_id, seq, kind, run_id, operator_id, text, evidence, note)
SELECT @finding_id, coalesce(max(seq) + 1, 0), @kind, sqlc.narg(run_id), sqlc.narg(operator_id),
    sqlc.narg(text), sqlc.narg(evidence), sqlc.narg(note)
FROM platform_agent_finding_events
WHERE finding_id = @finding_id;

-- name: ListFindings :many
SELECT f.id, a.name AS agent, f.status, f.title, f.cites, f.assignee_id,
    f.first_seen_at, f.last_seen_at, f.seen_count, f.status_changed_at
FROM platform_agent_findings f
JOIN platform_agents a ON a.id = f.agent_id
WHERE f.status = ANY (@statuses::text[])
ORDER BY f.last_seen_at DESC
LIMIT @row_limit;

-- name: CountFindingsByStatus :many
SELECT status, count(*)::integer AS findings
FROM platform_agent_findings
GROUP BY status;

-- name: GetFinding :one
SELECT f.id, a.name AS agent, f.status, f.title, f.cites, f.assignee_id,
    f.first_seen_at, f.last_seen_at, f.seen_count, f.status_changed_at
FROM platform_agent_findings f
JOIN platform_agents a ON a.id = f.agent_id
WHERE f.id = @id;

-- name: ListFindingEvents :many
SELECT seq, kind, run_id, operator_id, text, evidence, note, occurred_at
FROM platform_agent_finding_events
WHERE finding_id = @finding_id
ORDER BY seq;

-- name: GetPlatformAgentRunAgent :one
SELECT agent_id FROM platform_agent_runs WHERE id = @id;

-- name: ProposeAction :one
INSERT INTO platform_agent_proposals (agent_id, run_id, action, tier, reason, cites, preview, proposed_at, expires_at)
VALUES (@agent_id, @run_id, @action, @tier, @reason, @cites, @preview, @proposed_at, @expires_at)
RETURNING id;

-- name: LiveProposalExists :one
SELECT EXISTS (
    SELECT 1 FROM platform_agent_proposals WHERE action = @action AND status = ANY (@live::text[])
);

-- name: DecideProposal :one
UPDATE platform_agent_proposals
SET status = @status, decided_by = @decided_by, decided_at = now(), decision_note = @note,
    finished_at = CASE WHEN @closes::boolean THEN now() END
WHERE id = @id AND status = 'proposed' AND expires_at > now()
RETURNING id;

-- name: ExpireProposals :many
UPDATE platform_agent_proposals
SET status = 'expired', finished_at = now()
WHERE status = 'proposed' AND expires_at <= now()
RETURNING id;

-- name: ClaimApprovedProposal :one
WITH next AS (
    SELECT id FROM platform_agent_proposals
    WHERE status = 'approved' AND NOT EXISTS (SELECT 1 FROM platform_agent_brake)
    ORDER BY decided_at, id
    LIMIT 1 FOR UPDATE SKIP LOCKED
)
UPDATE platform_agent_proposals p
SET status = 'running', started_at = now()
FROM next WHERE p.id = next.id
RETURNING p.id, p.action;

-- name: AbandonStaleProposals :many
UPDATE platform_agent_proposals
SET status = 'failed', finished_at = now(), outcome = @outcome
WHERE status = 'running' AND started_at < now() - make_interval(secs => @lease_seconds::double precision)
RETURNING id;

-- name: FinishProposal :execrows
UPDATE platform_agent_proposals
SET status = @status, finished_at = now(), outcome = @outcome
WHERE id = @id AND status = 'running';

-- name: ListProposals :many
WITH eligible AS (
    SELECT p.id, a.name AS agent, p.action, p.tier, p.reason, p.status, p.proposed_at, p.expires_at, p.finished_at
    FROM platform_agent_proposals p
    JOIN platform_agents a ON a.id = p.agent_id
    WHERE p.status = ANY (@unbounded::text[])
        OR (p.status = ANY (@recent::text[]) AND p.finished_at >= @closed_since)
), ordered AS (
    SELECT *, 0 AS priority, expires_at AS pending_sort, NULL::timestamptz AS other_sort
    FROM eligible WHERE status = @pending_status
    UNION ALL
    SELECT *, 1 AS priority, NULL::timestamptz AS pending_sort,
        coalesce(finished_at, proposed_at) AS other_sort
    FROM eligible WHERE status <> @pending_status
)
SELECT id, agent, action, tier, reason, status, proposed_at, expires_at, finished_at,
    count(*) OVER() AS total
FROM ordered
ORDER BY priority, pending_sort ASC NULLS LAST, other_sort DESC NULLS LAST, id DESC
LIMIT @row_limit::int OFFSET @row_offset::int;

-- name: GetProposal :one
SELECT p.id, a.name AS agent, p.run_id, p.action, p.tier, p.reason, p.cites, p.preview, p.status,
    p.proposed_at, p.expires_at, p.decided_by, p.decided_at, p.decision_note,
    p.started_at, p.finished_at, p.outcome
FROM platform_agent_proposals p
JOIN platform_agents a ON a.id = p.agent_id
WHERE p.id = @id;
