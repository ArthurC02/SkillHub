---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L26-26"
    excerpt_sha256: "sha256:d8a1f6483675722f7ca9b4e5cb8339e2ddf3d25df55dd5a8e02da3bcca9e704d"
---
run_settings keeps check run_settings.singleton: singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton)

# Relations
- Contexts: [run](/contexts/run)
