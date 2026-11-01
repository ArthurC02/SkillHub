CREATE TABLE mcp_tools (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server        text NOT NULL CHECK (length(server) <= 64 AND server ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    tool          text NOT NULL CHECK (length(tool) <= 64 AND tool ~ '^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$'),
    purpose       text NOT NULL CHECK (btrim(purpose) <> '' AND length(purpose) <= 500),
    parameters    text NOT NULL CHECK (length(parameters) <= 1000),
    server_url    text NOT NULL CHECK (length(server_url) <= 500 AND server_url ~ '^https://[^\s/?#]+'),
    reason        text NOT NULL CHECK (btrim(reason) <> ''),
    created_at    timestamptz NOT NULL DEFAULT now(),
    retired_at    timestamptz,
    retire_reason text,
    CONSTRAINT retirement_is_explained CHECK ((retired_at IS NULL) = (retire_reason IS NULL)),
    CONSTRAINT retire_reason_is_not_blank CHECK (retire_reason IS NULL OR btrim(retire_reason) <> '')
);

CREATE UNIQUE INDEX mcp_tools_active_reference_key ON mcp_tools (server, tool) WHERE retired_at IS NULL;

COMMENT ON TABLE mcp_tools IS
    'The MCP tools operators allow Skills to declare. A Skill names one in allowed-tools as mcp__<server>__<tool>; a name not listed here is not a tool the platform offers.';
COMMENT ON COLUMN mcp_tools.server IS
    'The server part of the reference name: lowercase letters and digits in hyphen-separated words, so the double underscore of the reference never appears inside it.';
COMMENT ON COLUMN mcp_tools.tool IS
    'The tool name the server advertises, words joined by single underscores or hyphens.';
COMMENT ON COLUMN mcp_tools.purpose IS
    'What the tool does, written for the creator choosing it and for the creation model.';
COMMENT ON COLUMN mcp_tools.parameters IS
    'A summary of the tool''s parameters; empty when it takes none.';
COMMENT ON COLUMN mcp_tools.server_url IS
    'The HTTPS address of the server: what identifies the tool, since a name alone can be pointed anywhere.';
COMMENT ON COLUMN mcp_tools.reason IS
    'Why an operator added the tool.';
COMMENT ON COLUMN mcp_tools.retired_at IS
    'When an operator withdrew the tool; a retired row stays so the history of what was offered is kept.';
COMMENT ON COLUMN mcp_tools.retire_reason IS
    'Why the tool was withdrawn; present exactly when retired_at is.';
