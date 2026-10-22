---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0003_skill_registry.sql#L10-10"
    excerpt_sha256: "sha256:5a375150267c485168a9e28c0c9598c803b478bd08332b128306e9313682065b"
---
skill_sources keeps check skill_sources_git_needs_url: CONSTRAINT skill_sources_git_needs_url CHECK (source_type <> 'git' OR source_url IS NOT NULL)

# Relations
- Contexts: [ingest](/contexts/ingest)
