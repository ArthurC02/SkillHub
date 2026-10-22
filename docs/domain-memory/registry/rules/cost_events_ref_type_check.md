---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L30-31"
    excerpt_sha256: "sha256:bab5dd821e790091aaab9bea56501888a18411c81291a69f4e7c557000c9343a"
---
cost_events keeps check cost_events_ref_type_check: CONSTRAINT cost_events_ref_type_check CHECK (ref_type IS NULL OR ref_type IN ( 'creation_session', 'run', 'skill_version', 'platform_agent_run'))

# Relations
- Contexts: [credit](/contexts/credit)
