---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L22-23"
    excerpt_sha256: "sha256:405ece6cb4c198abf1997e3db2c7021dd2f3a4c68ea253e1763215e1031512d5"
---
platform_agent_finding_events keeps check platform_agent_finding_events.kind: kind text NOT NULL CHECK (kind IN ('opened', 'seen', 'reopened', 'recovered', 'acknowledged', 'resolved', 'dismissed'))

# Relations
- Contexts: [operations](/contexts/operations)
