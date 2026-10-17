---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L63-82"
    excerpt_sha256: "sha256:1eb3c302986474d39fd30cf0a8d8accbf8edaca932400154c969373113f75af8"
  - resource: "apps/platform/internal/product/learning/analytics_test.go#L119-154"
    excerpt_sha256: "sha256:cb3a43c72f0f00af1e09f9176968998d249493a0bbb6f2262a464b6f1b0110e6"
---
An analytics session cookie that is not a minted id is never used as the session id; it is replaced with a fresh one.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- A cookie holding an email address is not recorded; a new id is issued.
