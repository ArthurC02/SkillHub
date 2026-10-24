---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/http.go#L343-361"
    excerpt_sha256: "sha256:d49a43458e53ce6a7ea39de260a268a7adfbf3bb78274685b90fc119dea9ba40"
  - resource: "apps/platform/internal/skill/library/http_test.go#L13-50"
    excerpt_sha256: "sha256:4aa8e925c744c1e006dbff381f70c82e0bf4b1a0bfe973cd271faf03b8306da0"
---
The Skill listing accepts a limit of 1 to 100 and a non-negative offset; any other value, including an empty or non-integer one, is refused.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- limit=101 is refused.
