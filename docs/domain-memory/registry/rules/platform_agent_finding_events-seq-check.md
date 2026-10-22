---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L21-21"
    excerpt_sha256: "sha256:c23e886e424021aee86413d7b67c304cae5358a98f3e667fb975c37c73b8286e"
---
platform_agent_finding_events keeps check platform_agent_finding_events.seq: seq integer NOT NULL CHECK (seq >= 0)

# Relations
- Contexts: [operations](/contexts/operations)
