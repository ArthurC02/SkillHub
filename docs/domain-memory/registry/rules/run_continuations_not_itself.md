---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L11-11"
    excerpt_sha256: "sha256:5e59d204ed961a0e9a4713d9a144533bc3f5d1e5834ff951b2137c23783f16fa"
---
run_continuations keeps check run_continuations_not_itself: CONSTRAINT run_continuations_not_itself CHECK (run_id <> continues_run_id)

# Relations
- Contexts: [run](/contexts/run)
