---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L13-13"
    excerpt_sha256: "sha256:54c85b19f3974086d99cc752e6ade7325e3e8690e53dcc31458e6be6f9d62046"
---
platform_agent_findings keeps check platform_agent_findings:13: CHECK (status <> 'acknowledged' OR assignee_id IS NOT NULL)

# Relations
- Contexts: [operations](/contexts/operations)
