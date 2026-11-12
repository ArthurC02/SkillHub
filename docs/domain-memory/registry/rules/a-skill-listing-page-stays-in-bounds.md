---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T11:53:00.622260Z"
sources:
  - resource: "apps/platform/internal/skill/library/http.go#L339-347"
    excerpt_sha256: "sha256:6480b27b4034f5d16a204bc093051df18351de0f08e257c2cbc753a6c3b9ab55"
  - resource: "apps/platform/internal/skill/library/http_test.go#L13-50"
    excerpt_sha256: "sha256:4aa8e925c744c1e006dbff381f70c82e0bf4b1a0bfe973cd271faf03b8306da0"
---
The Skill listing accepts a limit of 1 to 100 and a non-negative offset; any other value, including an empty or non-integer one, is refused.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- limit=101 is refused.
