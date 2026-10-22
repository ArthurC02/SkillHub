---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0026_test_case_rubric.sql#L1-12"
    excerpt_sha256: "sha256:4cbe03967f54a89a8d4da892b70c9468665d0abfebc5c46535c9c4e9e35a47e3"
---
test_cases keeps check test_cases_rubric_shape: CONSTRAINT test_cases_rubric_shape CHECK ( rubric IS NULL OR ( jsonb_typeof(rubric -> 'version') = 'string' AND jsonb_typeof(rubric -> 'items') = 'array' AND NOT jsonb_path_exists( rubric, '$.items[*] ? (!exists(@.id) || !exists(@.text) ||…

# Relations
- Contexts: [testlab](/contexts/testlab)
