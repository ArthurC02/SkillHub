---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L18-21"
    excerpt_sha256: "sha256:ffa996b14083d38b5eb03ce08af6a55e66710a287569b8f6ee8a16245e2e428e"
---
cost_events keeps check cost_events_kind_check: CONSTRAINT cost_events_kind_check CHECK (kind IN ( 'creation_step', 'search_embedding', 'index_enrich', 'review', 'suggestion', 'generate', 'run', 'match_reasons', 'suggest_criteria', 'search_intent', 'platform_agent'))

# Relations
- Contexts: [credit](/contexts/credit)
