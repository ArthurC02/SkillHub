---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L21-23"
    excerpt_sha256: "sha256:951b1862f6bd42e7b230b66506d7a4546ed73055776975b4d88f2a38bbf87375"
---
evaluations keeps check evaluations_cost_needs_source: CONSTRAINT evaluations_cost_needs_source CHECK ((cost_usd IS NULL) = (cost_source IS NULL))

# Relations
- Contexts: [eval](/contexts/eval)
