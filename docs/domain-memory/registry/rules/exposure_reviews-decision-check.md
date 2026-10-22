---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0089_exposure_reviews.sql#L14-14"
    excerpt_sha256: "sha256:ab2ff596e7f40decf8643f89d01dcd7c1320a06450ae1777823f350a6581fefe"
---
exposure_reviews keeps check exposure_reviews.decision: decision text NOT NULL CHECK (decision IN ('approved', 'revoked'))

# Relations
- Contexts: [publishing](/contexts/publishing)
