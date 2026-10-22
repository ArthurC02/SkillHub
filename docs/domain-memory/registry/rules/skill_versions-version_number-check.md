---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0003_skill_registry.sql#L35-35"
    excerpt_sha256: "sha256:b5f650f379348563f2b4deebf9946b7e099a0538af47f6768abee89612590df2"
---
skill_versions keeps check skill_versions.version_number: version_number integer NOT NULL CHECK (version_number > 0)

# Relations
- Contexts: [registry](/contexts/registry)
