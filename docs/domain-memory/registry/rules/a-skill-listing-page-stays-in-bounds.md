---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T08:46:39.695261Z"
sources:
  - resource: "apps/platform/internal/skill/library/http.go#L340-355"
    excerpt_sha256: "sha256:dc30398f1eeda4d6f9adab4fabbe37c3237f54ec8720c0a438dbfc31866b7b7a"
  - resource: "apps/platform/internal/skill/library/http_test.go#L13-50"
    excerpt_sha256: "sha256:4aa8e925c744c1e006dbff381f70c82e0bf4b1a0bfe973cd271faf03b8306da0"
---
The Skill listing accepts a limit of 1 to 100 and a non-negative offset; any other value, including an empty or non-integer one, is refused.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- limit=101 is refused.
