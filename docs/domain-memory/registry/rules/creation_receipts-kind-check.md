---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L28-28"
    excerpt_sha256: "sha256:f62fec836cdf36ba314f745f84bf714aa2b435a159773c22f8c558f0353ecbf1"
---
creation_receipts keeps check creation_receipts.kind: kind text NOT NULL CHECK (kind IN ('command','attempt'))

# Relations
- Contexts: [creation](/contexts/creation)
