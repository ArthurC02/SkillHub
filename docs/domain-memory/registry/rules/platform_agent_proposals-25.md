---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L25-25"
    excerpt_sha256: "sha256:cb6f57525784ebf9665473ba045544bececca09aafbd919ebcc70624315d9916"
---
platform_agent_proposals keeps check platform_agent_proposals:25: CHECK ((status IN ('rejected', 'expired', 'succeeded', 'failed')) = (finished_at IS NOT NULL))

# Relations
- Contexts: [operations](/contexts/operations)
