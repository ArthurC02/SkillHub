---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/findings.go#L241-256"
    excerpt_sha256: "sha256:1cf92a482cba27ccfd4670b55ecb8f717572742b3494ca6977325bd59f818327"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_findings_integration_test.go#L203-246"
    excerpt_sha256: "sha256:b3130b26c0c0ff49e831718d1c095e31f7291b1686d4553ab417e3371c315ca8"
---
An operator may move a finding only along the allowed status paths; any other move is refused.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A dismissed finding cannot go straight to acknowledged, but can be reopened.
