---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0026_test_case_rubric.sql#L14-24"
    excerpt_sha256: "sha256:38f13b1abea387a09fc47415eb762f0773311ff4205f2f154b09489b16a9f4f5"
---
test_case_snapshots keeps check test_case_snapshots_rubric_shape: CONSTRAINT test_case_snapshots_rubric_shape CHECK ( rubric IS NULL OR ( jsonb_typeof(rubric -> 'version') = 'string' AND jsonb_typeof(rubric -> 'items') = 'array' AND NOT jsonb_path_exists( rubric, '$.items[*] ? (!exists(@.id) || !exists(@…

# Relations
- Contexts: [testlab](/contexts/testlab)
