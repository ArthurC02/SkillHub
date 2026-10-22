---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L5-5"
    excerpt_sha256: "sha256:bbb126d5ae0c9cd7bf50a476343d5883a215ea990f936398d970584ba73b3990"
---
platform_agent_steps keeps check platform_agent_steps.seq: seq integer NOT NULL CHECK (seq >= 0)

# Relations
- Contexts: [operations](/contexts/operations)
