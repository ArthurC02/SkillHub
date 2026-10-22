---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L12-12"
    excerpt_sha256: "sha256:84839a6f5511298686df86df35f3ddb888f0456c35636759c4c75b70de072429"
---
platform_agents keeps check platform_agents:12: CHECK (NOT enabled OR owner_id IS NOT NULL)

# Relations
- Contexts: [operations](/contexts/operations)
