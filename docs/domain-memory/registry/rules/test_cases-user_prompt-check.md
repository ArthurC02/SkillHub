---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L6-6"
    excerpt_sha256: "sha256:6f58636a4b0f01bbebc72db82cc42b46eb3285902c1505d60029cd9a724b39a5"
---
test_cases keeps check test_cases.user_prompt: user_prompt text NOT NULL CHECK (btrim(user_prompt) <> '')

# Relations
- Contexts: [testlab](/contexts/testlab)
