---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L37-38"
    excerpt_sha256: "sha256:8d89251470daeb870829ddab4b02348b0ee542d1b9ee1bc2ed1fc077d50a2c21"
---
evaluations keeps unique-index evaluations_current_key: CREATE UNIQUE INDEX evaluations_current_key ON evaluations (run_id) WHERE superseded_at IS NULL

# Relations
- Contexts: [eval](/contexts/eval)
