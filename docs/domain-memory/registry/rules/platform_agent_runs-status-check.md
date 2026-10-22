---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L25-26"
    excerpt_sha256: "sha256:175a954df3ce67d1b97cbde40540fb2b7ee6537b5f9a8b05e71afe7787c4d597"
---
platform_agent_runs keeps check platform_agent_runs.status: status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'incomplete', 'stopped', 'failed'))

# Relations
- Contexts: [operations](/contexts/operations)
