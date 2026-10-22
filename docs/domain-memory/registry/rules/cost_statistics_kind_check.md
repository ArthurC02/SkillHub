---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0111_platform_agent_steps.sql#L24-27"
    excerpt_sha256: "sha256:4050495e2f30f0efcada77ffcc9d765fb1ecd83c273ab5f77af801964551c63f"
---
cost_statistics keeps check cost_statistics_kind_check: CONSTRAINT cost_statistics_kind_check CHECK (kind IN ( 'creation_step', 'search_embedding', 'index_enrich', 'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons', 'suggest_criteria', 'search_intent', 'platform_agen…

# Relations
- Contexts: [credit](/contexts/credit)
