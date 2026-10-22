---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0130_skill_version_content_erasure.sql#L10-12"
    excerpt_sha256: "sha256:e5c8a89e3dc1317ad336110ce0aef9bc7fb05b0ce64c42c1a666f6d9a13312e2"
---
skill_versions keeps trigger skill_versions_immutable: CREATE TRIGGER skill_versions_immutable BEFORE DELETE ON skill_versions FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [registry](/contexts/registry)
