---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0116_operator_platform_settings.sql#L8-8"
    excerpt_sha256: "sha256:c47cb426cc5ea98fee973419191871fa086041cb9886fb6ccd0a1ca60cb54393"
---
evaluation_settings keeps check evaluation_settings.singleton: singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton)

# Relations
- Contexts: [eval](/contexts/eval)
