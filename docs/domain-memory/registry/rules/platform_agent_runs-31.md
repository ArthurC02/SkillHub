---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L31-31"
    excerpt_sha256: "sha256:3b4b0a093c9f864ab6ecc27fca9bab8f866e742e38b5893157a41b5366b56ae4"
---
platform_agent_runs keeps check platform_agent_runs:31: CHECK (finished_at IS NULL OR status = 'completed' OR btrim(reason) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
