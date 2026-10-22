---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L65-66"
    excerpt_sha256: "sha256:75dcf1900483096575d9adce01ee643331753c043ca8461e5ae9333873ac6f65"
---
evaluation_suggestions keeps check evaluation_suggestions.decision: decision text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending', 'accepted', 'rejected'))

# Relations
- Contexts: [eval](/contexts/eval)
