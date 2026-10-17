---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/http.go#L318-336"
    excerpt_sha256: "sha256:d49a43458e53ce6a7ea39de260a268a7adfbf3bb78274685b90fc119dea9ba40"
---
The Skill listing accepts a limit of 1 to 100 and a non-negative offset; any other value, including an empty or non-integer one, is refused.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- limit=101 is refused.
