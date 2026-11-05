---
type: Business Rule
status: stable
owner: skill-packaging
verified:
  - by: "human:ArthurC"
    at: "2026-11-05T16:50:27.141932Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/mcp.go#L53-94"
    excerpt_sha256: "sha256:872a48ec2296f1a904e17c8ac06fd29cfb782666d054a18882681e5dfaabdc04"
  - resource: "apps/platform/internal/skill/delivery/mcp.go#L96-99"
    excerpt_sha256: "sha256:6417deb6185471056734b21815654298633fcb5b3f988c5630f3e123c530dc99"
  - resource: "apps/platform/internal/skill/delivery/mcp.go#L101-123"
    excerpt_sha256: "sha256:20fc1406ded57646a279ba181f87b85f5b9d62dfa1c7ad7714c9a48426ae080d"
  - resource: "apps/platform/internal/skill/delivery/mcp_test.go#L80-115"
    excerpt_sha256: "sha256:239cbeb28077d3aae76684c6c6619916557dc21c08afa1c453fb7ad29d1adf3e"
  - resource: "apps/platform/internal/skill/delivery/mcp_test.go#L11-57"
    excerpt_sha256: "sha256:c6eeee8c1613c6778b5c72692409d8a003b6ececb87bb2fcdd790ca410bc0540"
---
An export names every MCP server its Skills declare in allowed-tools that is on the operator's MCP tool list: a plugin declares each one as a remote Streamable HTTP server in mcp.json and .mcp.json, a single Skill package lists them in INSTALL.md; a tool not on the list, a server listed under two addresses, or an address with credentials or a fragment is named for the reader to set up and never written, and an export whose tool list cannot be read is not produced.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A member declares mcp__docs__search and docs is listed at https://docs.example.com/mcp: mcp.json declares docs as streamable-http at that address and .mcp.json as http.
- A member declares mcp__crm__lookup, which is not on the list: no server is written for it and the README names it for the reader to set up.
- The MCP tool list cannot be read: no plugin is built.
