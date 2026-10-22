---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0016_run_orchestration.sql#L5-5"
    excerpt_sha256: "sha256:348c1846154540e539ccca8081c52a209a8ed2df5322a89d825846eecfa8c0ad"
---
run_attempts keeps check run_attempts.attempt_number: attempt_number integer NOT NULL CHECK (attempt_number > 0)

# Relations
- Contexts: [run](/contexts/run)
