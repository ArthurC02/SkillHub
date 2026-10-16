---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L331-333"
    excerpt_sha256: "sha256:33d1b4efbccd53ed84846906f4aea9d79e5cc90da265e559fd42498c55bce57c"
---
# Relations
- Producer context: [catalog](/contexts/catalog)
- Consumer context: [registry](/contexts/registry)
