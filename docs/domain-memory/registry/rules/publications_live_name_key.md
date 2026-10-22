---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0125_publication_name_release.sql#L5-5"
    excerpt_sha256: "sha256:eb03c9496c0490e526368217143aef617cc965bf3aa46ffcab7ab28f41996825"
---
publications keeps unique-index publications_live_name_key: CREATE UNIQUE INDEX publications_live_name_key ON publications (publisher_id, name) WHERE retired_at IS NULL

# Relations
- Contexts: [publishing](/contexts/publishing)
