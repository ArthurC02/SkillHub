---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/http.go#L150-150"
    excerpt_sha256: "sha256:219bae0a6b69660694667645c9f09403a77c7ec638131346d5617e3abeaf709b"
---
Every publishing request reads at most 64 KiB of JSON and refuses a longer body with 400.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A body of 64 KiB of spaces followed by valid JSON is refused.
