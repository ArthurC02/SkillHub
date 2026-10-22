---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L24-24"
    excerpt_sha256: "sha256:70b750069544b60874a696fb2f46a7b9405e6a075b1fe33e99a19e3a97b11a5c"
---
platform_agent_proposals keeps check platform_agent_proposals:24: CHECK ((status IN ('running', 'succeeded', 'failed')) = (started_at IS NOT NULL))

# Relations
- Contexts: [operations](/contexts/operations)
