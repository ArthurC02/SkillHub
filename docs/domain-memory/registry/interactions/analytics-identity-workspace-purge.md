---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L213-213"
    excerpt_sha256: "sha256:101f269cac55f7df989edf1c7dc2c82040fee75ddce602a015ac897bb9b05068"
---
# Relations
- Producer context: [analytics](/contexts/analytics)
- Consumer context: [identity](/contexts/identity)
