---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L4-4"
    excerpt_sha256: "sha256:f55e184e0b6e4cc3d23a714b9c3cec291ebe6057b311f662010d2c07d147ca69"
---
run_continuations keeps unique run_continuations.continues_run_id: continues_run_id uuid NOT NULL UNIQUE

# Relations
- Contexts: [run](/contexts/run)
