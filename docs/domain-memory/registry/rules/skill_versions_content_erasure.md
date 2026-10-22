---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0130_skill_version_content_erasure.sql#L18-22"
    excerpt_sha256: "sha256:1296dc6fc7b62dae7fb84a5b812d68ed4547b5b6a3e5bd57df989bce22d0219a"
---
skill_versions keeps trigger skill_versions_content_erasure: CREATE TRIGGER skill_versions_content_erasure BEFORE UPDATE ON skill_versions FOR EACH ROW WHEN (OLD.content_erased_at IS NULL AND NEW.content_erased_at IS NOT NULL) EXECUTE FUNCTION enforce_immutable('manifest', 'source_id', 'content_eras…

# Relations
- Contexts: [registry](/contexts/registry)
