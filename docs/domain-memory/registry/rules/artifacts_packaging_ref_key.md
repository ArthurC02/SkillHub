---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0027_packaging.sql#L8-8"
    excerpt_sha256: "sha256:edbc2a6b33f83d09e0d3709632f0ea50091e47f80c6e2357e60555df66a514ec"
---
artifacts keeps unique-index artifacts_packaging_ref_key: CREATE UNIQUE INDEX artifacts_packaging_ref_key ON artifacts (id, kind, workspace_id)

# Relations
- Contexts: [run](/contexts/run)
