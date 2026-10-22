---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0016_run_orchestration.sql#L15-15"
    excerpt_sha256: "sha256:95bdd9ef928d2c4563dbc01b4a8af37d23a2161c66d0a0729b50f6537ff351a3"
---
run_attempts keeps unique-index run_attempts_run_number_key: CREATE UNIQUE INDEX run_attempts_run_number_key ON run_attempts (run_id, attempt_number)

# Relations
- Contexts: [run](/contexts/run)
