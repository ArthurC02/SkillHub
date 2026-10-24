---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L878-882"
    excerpt_sha256: "sha256:40c35de0bd0736573ccb035a2d38728b1daf2b94028543642c520b3bc329f5c1"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/packaging_integration_test.go#L1262-1308"
    excerpt_sha256: "sha256:2453b3957dbcb8ab80d730a94833de1bbb4b458720f50d5ef3853284e34febc1"
---
A package expires exactly the configured retention after it is built.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- With a seven-day retention, a package built now expires in seven days.
