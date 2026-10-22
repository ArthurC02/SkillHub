---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L73-73"
    excerpt_sha256: "sha256:14674e31833a8a94e9c20d14bb34e711eaef8be1838c15ddbf71063b307a3923"
---
runs keeps check runs.attempt: attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0)

# Relations
- Contexts: [run](/contexts/run)
