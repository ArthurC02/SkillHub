---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0103_workspace_scoped_foreign_keys.sql#L3-3"
    excerpt_sha256: "sha256:b59d23ca44723a8053144e27db975b990de5c23f59d9c43018174c958fb5c5bd"
---
test_case_snapshots keeps unique test_case_snapshots_id_workspace_key: CONSTRAINT test_case_snapshots_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [testlab](/contexts/testlab)
