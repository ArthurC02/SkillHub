---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T06:02:14.778167Z"
sources:
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools.go#L76-95"
    excerpt_sha256: "sha256:3e033fa7b3d80002f5fe5fbf2eba61d9b2e110db67863b5289c407515a8c7873"
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools.go#L151-178"
    excerpt_sha256: "sha256:63ea4fbcaba89cd881824066a2f556b9529023dba85ece6a62012c4e4c36ceed"
  - resource: "db/migrations/0147_mcp_tool_catalog.sql#L1-16"
    excerpt_sha256: "sha256:6dcc462bc595155f8606d4ea22e96c4b37237347100b1ccdb11b5cbda9b1e638"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/mcp_tools_integration_test.go#L36-58"
    excerpt_sha256: "sha256:b0ab5c69924fa97a6c4b81db9b09993c2e7b698f5e509d84fed763a06e4cc23f"
---
An operator adds a row to mcp_tools with a server, tool, purpose, parameters, an HTTPS server_url and a reason, written in one transaction with its audit event; at most one unretired row exists per server and tool, and a second is refused as a conflict.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Adding calendar/create_event twice without retiring the first answers 409 the second time.
- A draft with an http address or an uppercase server name is refused before anything is listed.
