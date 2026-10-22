---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L5-5"
    excerpt_sha256: "sha256:489a7c335f3b29f294a8389430e116806d4005c9e02463062ac39e5155c9e5d7"
---
platform_agent_proposals keeps check platform_agent_proposals.action: action text NOT NULL CHECK (btrim(action) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
