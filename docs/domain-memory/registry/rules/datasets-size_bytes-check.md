---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L21-21"
    excerpt_sha256: "sha256:e8361d8564b40ea58c3f04c94822f6c0de2fdd32702fa7e88347ea3ec452f226"
---
datasets keeps check datasets.size_bytes: size_bytes bigint NOT NULL CHECK (size_bytes >= 0)

# Relations
- Contexts: [testlab](/contexts/testlab)
