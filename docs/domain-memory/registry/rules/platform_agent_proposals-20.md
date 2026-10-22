---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L20-20"
    excerpt_sha256: "sha256:f86179485f08f62d8c050f3e0fac560eea80ca0bfe7508af9aa241fba27c6dcc"
---
platform_agent_proposals keeps check platform_agent_proposals:20: CHECK (expires_at > proposed_at)

# Relations
- Contexts: [operations](/contexts/operations)
