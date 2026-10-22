---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0122_run_origin_key.sql#L4-4"
    excerpt_sha256: "sha256:792aae7519cda07a0132cf425ee891066d0a5149667d632f4f5187ad7c636723"
---
runs keeps unique-index runs_workspace_origin_key: CREATE UNIQUE INDEX runs_workspace_origin_key ON runs (workspace_id, origin_key) WHERE origin_key IS NOT NULL

# Relations
- Contexts: [run](/contexts/run)
