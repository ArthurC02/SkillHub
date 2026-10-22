---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0089_exposure_reviews.sql#L15-15"
    excerpt_sha256: "sha256:c8b97fb089ec26b4ee600868adbdb2ab776c8e38728c599778d9fd00f01b0407"
---
exposure_reviews keeps check exposure_reviews.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [publishing](/contexts/publishing)
