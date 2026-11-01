-- name: ListMCPTools :many
SELECT * FROM mcp_tools
ORDER BY retired_at IS NOT NULL, server, tool, created_at DESC;

-- name: ListActiveMCPTools :many
SELECT * FROM mcp_tools
WHERE retired_at IS NULL
ORDER BY server, tool;

-- name: InsertMCPTool :one
INSERT INTO mcp_tools (server, tool, purpose, parameters, server_url, reason)
VALUES (@server, @tool, @purpose, @parameters, @server_url, @reason)
RETURNING *;

-- name: RetireMCPTool :one
UPDATE mcp_tools
SET retired_at = now(), retire_reason = @retire_reason
WHERE id = @id AND retired_at IS NULL
RETURNING *;
