---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L4-5"
    excerpt_sha256: "sha256:31f2d5cb3509b19f9297b8c5d0f895c61e6777c1437d1ae53e629e219d06b342"
---
platform_agent_findings keeps check platform_agent_findings.status: status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved', 'dismissed', 'recovered'))

# Relations
- Contexts: [operations](/contexts/operations)
