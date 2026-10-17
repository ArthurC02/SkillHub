---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/findings.go#L241-256"
    excerpt_sha256: "sha256:1cf92a482cba27ccfd4670b55ecb8f717572742b3494ca6977325bd59f818327"
---
An operator may move a finding only along the allowed status paths; any other move is refused.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A dismissed finding cannot go straight to acknowledged, but can be reopened.
