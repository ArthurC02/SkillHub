---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0017_test_case_deletion.sql#L6-6"
    excerpt_sha256: "sha256:805c423c1cae4ccf5090589358b45d0f439fba62e83c4b53603916674445b666"
---
test_cases keeps check test_cases_name_not_blank: CONSTRAINT test_cases_name_not_blank CHECK (btrim(name) <> '')

# Relations
- Contexts: [testlab](/contexts/testlab)
