---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0006_auth.sql#L9-10"
    excerpt_sha256: "sha256:cacbb2881a520e15155fc2a893c486772f3a1e7325fed599fc1894f19b76f2f1"
---
user_identities keeps unique-index user_identities_user_provider_key: CREATE UNIQUE INDEX user_identities_user_provider_key ON user_identities (user_id, provider)

# Relations
- Contexts: [identity](/contexts/identity)
