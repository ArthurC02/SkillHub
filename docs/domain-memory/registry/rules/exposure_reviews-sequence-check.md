---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0089_exposure_reviews.sql#L10-10"
    excerpt_sha256: "sha256:1d9a72e269303b9ea1125ebb840885f07c981fefcbbd9a6b5f7c521d1775f272"
---
exposure_reviews keeps check exposure_reviews.sequence: sequence integer NOT NULL CHECK (sequence > 0)

# Relations
- Contexts: [publishing](/contexts/publishing)
