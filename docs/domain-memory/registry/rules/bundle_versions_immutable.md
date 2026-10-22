---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L35-37"
    excerpt_sha256: "sha256:2d47eea8960edf0e78ec22fff6120dc038b37b2060bb741ba98929f9bfa16b45"
---
bundle_versions keeps trigger bundle_versions_immutable: CREATE TRIGGER bundle_versions_immutable BEFORE UPDATE OR DELETE ON bundle_versions FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [publishing](/contexts/publishing)
