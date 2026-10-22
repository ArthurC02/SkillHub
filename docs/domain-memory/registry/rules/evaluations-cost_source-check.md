---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L1-19"
    excerpt_sha256: "sha256:bdd996efce2b7dd3adb0cfb8c65495e44bfd2113adf296b116ea1dba4bad80ad"
---
evaluations keeps check evaluations.cost_source: cost_source text CHECK (cost_source IN ('gateway', 'estimated'))

# Relations
- Contexts: [eval](/contexts/eval)
