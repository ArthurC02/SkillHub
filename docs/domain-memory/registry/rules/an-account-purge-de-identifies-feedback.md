---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/purge.go#L12-19"
    excerpt_sha256: "sha256:d99774fae07a4d8b414657d86781d19459d5d76f3055857ee5a1b0a1203df9ae"
---
Purging an account detaches its feedback reports and funnel events from the Workspace and user, keeping the text, rather than deleting them.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- After the purge, the report text remains with no Workspace or user.
