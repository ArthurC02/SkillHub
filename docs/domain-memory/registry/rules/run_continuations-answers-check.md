---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L6-6"
    excerpt_sha256: "sha256:553b08e0f44a9906aeff2ae8ebb9b781257e43dea5449c8a993c1384ac1d90b9"
---
run_continuations keeps check run_continuations.answers: answers jsonb NOT NULL CHECK (jsonb_typeof(answers) = 'array')

# Relations
- Contexts: [run](/contexts/run)
