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
SET status = @status, finished_at = now(), reason = sqlc.narg(reason)
WHERE id = @id AND status = 'running';
