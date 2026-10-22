---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L7-7"
    excerpt_sha256: "sha256:089f4aa09a1b6b21cd422d2119f49526334407df94b9f035bf30fd4b5be1b979"
---
platform_agent_findings keeps check platform_agent_findings.cites: cites text[] NOT NULL CHECK (cardinality(cites) > 0)

# Relations
- Contexts: [operations](/contexts/operations)
