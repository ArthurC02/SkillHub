---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L16-16"
    excerpt_sha256: "sha256:04a76a364ce7177a3cdf5ca4fa0efe718ad9d9d9f62c7c8744b481e4b22feb80"
---
platform_agent_brake keeps check platform_agent_brake.engaged: engaged boolean PRIMARY KEY DEFAULT true CHECK (engaged)

# Relations
- Contexts: [operations](/contexts/operations)
