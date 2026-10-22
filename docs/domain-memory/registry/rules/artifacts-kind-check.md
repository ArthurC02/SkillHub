---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L144-144"
    excerpt_sha256: "sha256:4f7b6a5bc50e533f28858fc14bef3a1162e569eb83ac1a0bad71b9d79aa29f7e"
---
artifacts keeps check artifacts.kind: kind text NOT NULL CHECK (kind IN ('run_output', 'download_package'))

# Relations
- Contexts: [run](/contexts/run)
