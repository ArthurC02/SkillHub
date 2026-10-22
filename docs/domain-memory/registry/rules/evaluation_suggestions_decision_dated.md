---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L71-72"
    excerpt_sha256: "sha256:7ec9659fc4d98e9f680cc06b2f3226a6d6abe50a5b5deb679345fda9967cf409"
---
evaluation_suggestions keeps check evaluation_suggestions_decision_dated: CONSTRAINT evaluation_suggestions_decision_dated CHECK ((decision = 'pending') = (decided_at IS NULL))

# Relations
- Contexts: [eval](/contexts/eval)
