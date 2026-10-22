---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0127_account_erasure_expedited.sql#L3-5"
    excerpt_sha256: "sha256:b3885e510beaa86af3a47d88f2965d04ec09a38e35331a547b3da6752adde2e0"
---
users keeps check users_erasure_expedited_needs_request: CONSTRAINT users_erasure_expedited_needs_request CHECK (erasure_expedited_at IS NULL OR deletion_requested_at IS NOT NULL)

# Relations
- Contexts: [identity](/contexts/identity)
