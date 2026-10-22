---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L31-31"
    excerpt_sha256: "sha256:916409f2f61c05afdb2e9abdaf655a9a395b18a5c5b5a5f5b0658c7d4caf294b"
---
platform_agent_finding_events keeps check platform_agent_finding_events:31: CHECK (num_nonnulls(run_id, operator_id) = 1)

# Relations
- Contexts: [operations](/contexts/operations)
