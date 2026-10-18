---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/analytics.go#L216-218"
    excerpt_sha256: "sha256:9678ff58dfbf0b671b58ac038f3b4d705870a2dafee371e3f510076d47d9284e"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trends_integration_test.go#L244-298"
    excerpt_sha256: "sha256:d44df36ed33b88d976120154604575964a17d281d9c6836549af73edeaf5ce6f"
---
The daily funnel counts downloads as distinct Workspaces and every other stage as distinct sessions.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Two sessions of one Workspace downloading count as one download.
