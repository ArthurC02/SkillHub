---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L39-41"
    excerpt_sha256: "sha256:a790a20ae5d7d0370c5fc12f024b9a04e5e976916e8998dadd6eafa760cda7df"
---
bundle_members keeps trigger bundle_members_immutable: CREATE TRIGGER bundle_members_immutable BEFORE UPDATE OR DELETE ON bundle_members FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [publishing](/contexts/publishing)
