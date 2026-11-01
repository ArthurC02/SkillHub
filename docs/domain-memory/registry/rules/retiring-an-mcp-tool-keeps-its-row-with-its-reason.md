---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/mcptools/mcptools.go#L97-115"
    excerpt_sha256: "sha256:f187894956f43d6a02bcaba506684807ce3824c862b3ebc18f496aa22efd40ad"
  - resource: "db/migrations/0147_mcp_tool_catalog.sql#L11-16"
    excerpt_sha256: "sha256:6d250dee7144501cff9293d9582611a400a91f62c236b9e0038c2124c503425d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/mcp_tools_integration_test.go#L85-117"
    excerpt_sha256: "sha256:15ce2b04f93c5831962eec0a189645586a552237d56aa5dd8abeb6bb5ec2d640"
---
An operator retires an mcp_tools row with a non-blank reason, written with its audit event in one transaction; the row stays listed with its retirement reason, a blank reason is refused, and the retired row no longer occupies its server and tool.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Retiring a tool with the note the pilot ended lists it with retire_reason the pilot ended, and the same server and tool can be added again.
- Retiring with a blank note answers 400.
