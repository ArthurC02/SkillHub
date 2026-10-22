---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0037_generated_skill_provenance.sql#L29-31"
    excerpt_sha256: "sha256:716ffc3c14b61620105d6c04eb2a46003d4f2e9605b0f87af7a344b371eb06b8"
---
skills keeps check skills_redistribution_check: CONSTRAINT skills_redistribution_check CHECK (redistribution IN ('allowed', 'blocked', 'unknown', 'self_supplied', 'generated'))

# Relations
- Contexts: [registry](/contexts/registry)
