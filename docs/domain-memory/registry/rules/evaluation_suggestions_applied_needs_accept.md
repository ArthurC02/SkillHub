---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L73-74"
    excerpt_sha256: "sha256:4840f5456cf8b38915dd77b28e29f89ae2a144d88cbe9a2a450712b6c91cab46"
---
evaluation_suggestions keeps check evaluation_suggestions_applied_needs_accept: CONSTRAINT evaluation_suggestions_applied_needs_accept CHECK (applied_skill_version_id IS NULL OR decision = 'accepted')

# Relations
- Contexts: [eval](/contexts/eval)
