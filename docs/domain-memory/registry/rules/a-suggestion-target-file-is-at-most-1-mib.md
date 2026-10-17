---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L35-35"
    excerpt_sha256: "sha256:e8102a52cf248cb2f57c4663818b428e7ff0cf344c5f60b8e8275459a466b76d"
  - resource: "apps/platform/internal/trial/improvement/apply.go#L219-221"
    excerpt_sha256: "sha256:387c2ba24e0a9768ded1c7997f07c973ed0e8afaa906ae7f41f86ed8e8f51b0c"
---
A file a suggestion targets is read for a reviewable diff only up to 1 MiB; a larger file is refused before any content check.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A file of exactly 1048576 bytes is read; 1048577 bytes is refused as larger than the byte limit for a reviewable diff.
