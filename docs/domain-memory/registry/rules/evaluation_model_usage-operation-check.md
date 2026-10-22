---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0033_evaluation_model_usage.sql#L8-8"
    excerpt_sha256: "sha256:911ef30475a35fe5a4aa1d4e9cc486def9d8092815c146714791a85f9694892e"
---
evaluation_model_usage keeps check evaluation_model_usage.operation: operation text NOT NULL CHECK (operation IN ('judge', 'suggest'))

# Relations
- Contexts: [eval](/contexts/eval)
