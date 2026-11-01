---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T06:02:14.778167Z"
sources:
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools.go#L49-51"
    excerpt_sha256: "sha256:9be60cab1cb2c286718201a3a634166ee3e0e41582c4fed90a0346beb8b0b949"
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools.go#L137-149"
    excerpt_sha256: "sha256:89399023887ea21ff9ff504c50787c3b7d7c4d227d752cfccd2ceb74af6e6f52"
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools_test.go#L18-38"
    excerpt_sha256: "sha256:a6a54c5f145a83227d92597249a12a3ed1dafa19db76a772fbd9099948e1236d"
---
A Skill may declare in allowed-tools only MCP tools that operators list in mcp_tools, referenced as mcp__<server>__<tool>; an mcp__ name that no unretired mcp_tools row provides is unlisted, and a name without the mcp__ prefix is never judged against the list.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- mcp__calendar__create_event is accepted while mcp_tools lists calendar and create_event, and mcp__calendar__create_events is unlisted.
- Read and Bash(git:*) are not MCP names and are never unlisted.
