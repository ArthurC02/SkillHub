---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L21-21"
    excerpt_sha256: "sha256:20b014012fea3906b25c291c797e73bda5db121bf5399c4c483a7e9908aadc6c"
---
platform_agent_proposals keeps check platform_agent_proposals:21: CHECK ((status IN ('proposed', 'expired')) = (decided_by IS NULL))

# Relations
- Contexts: [operations](/contexts/operations)
