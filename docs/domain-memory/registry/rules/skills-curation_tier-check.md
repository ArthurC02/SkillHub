---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0042_curation_tier.sql#L1-6"
    excerpt_sha256: "sha256:6051b10aa526c540653457e294b893cf9a49f044b87fdbc21379218858bab36d"
---
skills keeps check skills.curation_tier: curation_tier text NOT NULL DEFAULT 'indexed' CHECK (curation_tier IN ('curated', 'indexed'))

# Relations
- Contexts: [registry](/contexts/registry)
