---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0124_skill_takedown_restore.sql#L42-43"
    excerpt_sha256: "sha256:4330174189f0f658d1a17ebdb1ac7b065e2dfb9654fa1e376ce618a7d2209456"
---
skills keeps check skills.takedown_by: takedown_by text CHECK (takedown_by IN ('owner', 'operator'))

# Relations
- Contexts: [registry](/contexts/registry)
