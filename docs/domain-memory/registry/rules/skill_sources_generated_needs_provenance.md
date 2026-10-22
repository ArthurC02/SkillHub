---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0037_generated_skill_provenance.sql#L10-19"
    excerpt_sha256: "sha256:a12d44e38818723bd643a17204bdffcb0f5fd26dd38c33f3195b0032e056acdf"
---
skill_sources keeps check skill_sources_generated_needs_provenance: CONSTRAINT skill_sources_generated_needs_provenance CHECK ( source_type <> 'generated' OR ( task_description IS NOT NULL AND generator_model IS NOT NULL AND generator_prompt_version IS NOT NULL ) )

# Relations
- Contexts: [ingest](/contexts/ingest)
