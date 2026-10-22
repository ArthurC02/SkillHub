---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0056_creation_sessions.sql#L5-5"
    excerpt_sha256: "sha256:3b9acbcc117c0ec43b138f78a126b8edca0aa8dee1ae4857426f2afb251afcf5"
---
creation_sessions keeps check creation_sessions.revision: revision bigint NOT NULL CHECK (revision >= 1)

# Relations
- Contexts: [creation](/contexts/creation)
