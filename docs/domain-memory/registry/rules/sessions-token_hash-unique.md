---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0006_auth.sql#L15-15"
    excerpt_sha256: "sha256:40781b0219d6b2878a98d68e1c91aad54280183a1d1bb323366d8de7e25cc0f0"
---
sessions keeps unique sessions.token_hash: token_hash bytea NOT NULL UNIQUE

# Relations
- Contexts: [identity](/contexts/identity)
