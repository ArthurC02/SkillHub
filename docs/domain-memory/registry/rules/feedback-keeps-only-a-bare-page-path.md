---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L143-149"
    excerpt_sha256: "sha256:1cd6327a8adb96c69060ac8d044373c93acf2d7b5a8033f66d82bee908aebb69"
---
A reported page path is kept only when it starts with '/', carries no query or fragment and is at most 512 bytes; otherwise it is dropped and the report is still accepted.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- /skills?q=secret is stored as no page path.
