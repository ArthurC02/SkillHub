---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:12:31.935298Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/scan.go#L11-13"
    excerpt_sha256: "sha256:075fad83956c729e76d872ca8826f0de58d9f6da0ec7ae631016f1383bfd8c9c"
  - resource: "apps/platform/internal/skill/delivery/scan_test.go#L12-48"
    excerpt_sha256: "sha256:1c6fbf00d595c149d835c3c269cb6b39ad060c31f19ff833d357f404b4666f81"
---
Packaging the same content again reuses an existing package only while it is available, not deleted, not purged and not expired; otherwise a new one is built.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A purged package with the same identity is not reused.
