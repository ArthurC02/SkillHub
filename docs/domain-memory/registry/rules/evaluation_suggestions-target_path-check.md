---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L58-61"
    excerpt_sha256: "sha256:c9ab08a445be2a36f5ab06ffed78d1c7250ffdb5609df5868f1a7080492da0fc"
---
evaluation_suggestions keeps check evaluation_suggestions.target_path: target_path text NOT NULL CHECK (target_path <> '' AND left(target_path, 1) <> '/' AND target_path !~ '(^|/)\.\.(/|$)')

# Relations
- Contexts: [eval](/contexts/eval)
