---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L56-56"
    excerpt_sha256: "sha256:0d676074ef2fc1b1125c402051b15dfc54623df4970590fe56fea7cf155c74ea"
---
evaluation_suggestions keeps check evaluation_suggestions.problem: problem text NOT NULL CHECK (btrim(problem) <> '')

# Relations
- Contexts: [eval](/contexts/eval)
