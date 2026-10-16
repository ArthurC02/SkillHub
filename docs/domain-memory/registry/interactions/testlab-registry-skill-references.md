---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L210-210"
    excerpt_sha256: "sha256:9540d7fc0fde98c93bc44f7003339e28665656c3b58d10360b8ab39c069504a8"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [registry](/contexts/registry)
