---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0130_skill_version_content_erasure.sql#L13-17"
    excerpt_sha256: "sha256:2339346077ffefabcbb9c413b2640e06e4b1499880dc32918d5edb14c955faaf"
---
skill_versions keeps trigger skill_versions_frozen: CREATE TRIGGER skill_versions_frozen BEFORE UPDATE ON skill_versions FOR EACH ROW WHEN (OLD.content_erased_at IS NOT NULL OR NEW.content_erased_at IS NULL) EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [registry](/contexts/registry)
