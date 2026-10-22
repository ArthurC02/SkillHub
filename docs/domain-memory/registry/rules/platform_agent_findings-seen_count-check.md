---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L11-11"
    excerpt_sha256: "sha256:ffdbfdefde8394b4a917261a7ac6cf1d680bad08ca02b93988cecb17c3cc6b3d"
---
platform_agent_findings keeps check platform_agent_findings.seen_count: seen_count integer NOT NULL DEFAULT 1 CHECK (seen_count > 0)

# Relations
- Contexts: [operations](/contexts/operations)
