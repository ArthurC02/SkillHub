---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0002_identity.sql#L10-10"
    excerpt_sha256: "sha256:ce15d1b6db0060be01bb9fc4b2314759688e7277cd0f09c38d9267520d859176"
---
users keeps unique-index users_email_key: CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE deleted_at IS NULL

# Relations
- Contexts: [identity](/contexts/identity)
