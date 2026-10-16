---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L214-214"
    excerpt_sha256: "sha256:101f269cac55f7df989edf1c7dc2c82040fee75ddce602a015ac897bb9b05068"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L49-49"
    excerpt_sha256: "sha256:94e9dea507f50aff153aeef705a9cd9b10532fdea612161b75a2d515fd52a3e5"
---
# Relations
- Producer context: [analytics](/contexts/analytics)
- Consumer context: [identity](/contexts/identity)
