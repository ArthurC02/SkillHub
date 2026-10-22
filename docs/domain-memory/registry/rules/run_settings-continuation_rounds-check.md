---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L27-27"
    excerpt_sha256: "sha256:325e46fb2b0703cef0a3c12c34b63b4d34346b158d3b9c6d8deeca9a851afb73"
---
run_settings keeps check run_settings.continuation_rounds: continuation_rounds integer NOT NULL CHECK (continuation_rounds BETWEEN 1 AND 10)

# Relations
- Contexts: [run](/contexts/run)
