---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L18-18"
    excerpt_sha256: "sha256:a50ac557d8fd39352e1c8f573b45aa8d7896098a8576eecfac35dd5ef16a0da4"
---
evaluation_grader_versions keeps check evaluation_grader_versions.version: version integer NOT NULL CHECK (version >= 1)

# Relations
- Contexts: [eval](/contexts/eval)
