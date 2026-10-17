---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:44:34.068147Z"
sources:
  - resource: "apps/platform/internal/product/learning/purge.go#L12-19"
    excerpt_sha256: "sha256:d99774fae07a4d8b414657d86781d19459d5d76f3055857ee5a1b0a1203df9ae"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L568-620"
    excerpt_sha256: "sha256:2d84118f2837268e9b80d1e828958b173762bc91e0a10d8a32c8fc0e6cb0bfde"
---
Purging an account detaches its feedback reports and funnel events from the Workspace and user, keeping the text, rather than deleting them.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- After the purge, the report text remains with no Workspace or user.
