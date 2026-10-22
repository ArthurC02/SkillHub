---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0087_publishing.sql#L34-36"
    excerpt_sha256: "sha256:47bbdcda3c3159ec16aeee14db92a249bb0fec2b2e9a5221c975003ee03a12f5"
---
publication_releases keeps trigger publication_releases_immutable: CREATE TRIGGER publication_releases_immutable BEFORE UPDATE OR DELETE ON publication_releases FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [publishing](/contexts/publishing)
