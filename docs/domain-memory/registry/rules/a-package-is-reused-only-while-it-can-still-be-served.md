---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/scan.go#L11-13"
    excerpt_sha256: "sha256:075fad83956c729e76d872ca8826f0de58d9f6da0ec7ae631016f1383bfd8c9c"
---
Packaging the same content again reuses an existing package only while it is available, not deleted, not purged and not expired; otherwise a new one is built.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A purged package with the same identity is not reused.
