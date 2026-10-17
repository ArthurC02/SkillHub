---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L63-82"
    excerpt_sha256: "sha256:1eb3c302986474d39fd30cf0a8d8accbf8edaca932400154c969373113f75af8"
---
An analytics session cookie that is not a minted id is never used as the session id; it is replaced with a fresh one.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A cookie holding an email address is not recorded; a new id is issued.
