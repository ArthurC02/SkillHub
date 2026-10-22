---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L19-19"
    excerpt_sha256: "sha256:b5af738bad1cf6f235ffe17bbade023e2efc768066aff47cf0729e3097d396e6"
---
platform_agent_brake keeps check platform_agent_brake.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
