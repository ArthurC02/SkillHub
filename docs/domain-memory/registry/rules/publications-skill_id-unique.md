---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L12-12"
    excerpt_sha256: "sha256:74aca922674b15a76416d82edf491be96d4b5890bf52f67d87ac73c726d836ec"
---
publications keeps unique publications.skill_id: skill_id uuid NOT NULL UNIQUE REFERENCES skills (id) ON DELETE CASCADE

# Relations
- Contexts: [publishing](/contexts/publishing)
