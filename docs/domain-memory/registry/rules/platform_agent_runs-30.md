---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L30-30"
    excerpt_sha256: "sha256:cf1b7c7c451456b085c84922fef6d094579130823206202d30a7c092380ce5dd"
---
platform_agent_runs keeps check platform_agent_runs:30: CHECK ((status = 'running') = (finished_at IS NULL))

# Relations
- Contexts: [operations](/contexts/operations)
