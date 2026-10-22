---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L22-22"
    excerpt_sha256: "sha256:551dcb8689c9a6a7b4d1887d750c14f7ff0cf3fee097c4f1e833985c79d6bcc3"
---
platform_agent_proposals keeps check platform_agent_proposals:22: CHECK ((decided_by IS NULL) = (decided_at IS NULL))

# Relations
- Contexts: [operations](/contexts/operations)
