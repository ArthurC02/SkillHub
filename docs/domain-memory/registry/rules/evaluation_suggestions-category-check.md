---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L53-54"
    excerpt_sha256: "sha256:744a7393e13e6b6a9d453d19f08f9233ea871f427ce34a3e4810a80afa55a155"
---
evaluation_suggestions keeps check evaluation_suggestions.category: category text NOT NULL CHECK (category IN ('skill', 'runtime', 'mcp', 'tool', 'dataset'))

# Relations
- Contexts: [eval](/contexts/eval)
