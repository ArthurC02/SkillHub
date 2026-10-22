---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0063_credit_session_summaries.sql#L6-6"
    excerpt_sha256: "sha256:e88fb6918caf3ab2511e630fcf4d1bb2530b27fc22012acc208e3bab5ca71e55"
---
cost_session_summaries keeps check cost_session_summaries.steps: steps integer NOT NULL CHECK (steps >= 1)

# Relations
- Contexts: [credit](/contexts/credit)
