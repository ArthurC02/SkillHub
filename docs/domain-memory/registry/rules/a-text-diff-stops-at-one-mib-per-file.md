---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T19:36:24.856168Z"
sources:
  - resource: "apps/platform/internal/skill/library/diff.go#L136-139"
    excerpt_sha256: "sha256:e0cfc6f388e76452a82c3d81ce58b35850f8488420e113c44b8b0444aa33695b"
  - resource: "apps/platform/internal/skill/library/content_guards_test.go#L77-85"
    excerpt_sha256: "sha256:c9ae40206ec9e25a98e073da3be1e249c9d274e438269e7f640daffd8d8037a6"
---
A file larger than 1 MiB on either side gets no text diff; exactly 1 MiB is diffed.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Two 1 MiB files are diffed; a file one byte over is not.
