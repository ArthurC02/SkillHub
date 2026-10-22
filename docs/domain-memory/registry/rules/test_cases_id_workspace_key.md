---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0103_workspace_scoped_foreign_keys.sql#L2-2"
    excerpt_sha256: "sha256:fd5cc14a61daf7d3df5bf5594bcd1eb7f7bd09f5b481c221716bac9c41e551bd"
---
test_cases keeps unique test_cases_id_workspace_key: CONSTRAINT test_cases_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [testlab](/contexts/testlab)
