---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0089_exposure_reviews.sql#L18-18"
    excerpt_sha256: "sha256:91bffe92a940c8e264eab931f6a4b77b8d20243315085df743e507f3ac2227fe"
---
exposure_reviews keeps unique exposure_reviews:18: UNIQUE (publication_id, sequence)

# Relations
- Contexts: [publishing](/contexts/publishing)
