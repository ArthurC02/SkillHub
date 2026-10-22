---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L20-20"
    excerpt_sha256: "sha256:0a891e1c9d63f8a93c1b35e9ace5e14fe643492b1eb8d0b7308672addfa6600c"
---
evaluation_grader_versions keeps check evaluation_grader_versions.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [eval](/contexts/eval)
