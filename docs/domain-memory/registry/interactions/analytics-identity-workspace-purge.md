---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L218-218"
    excerpt_sha256: "sha256:101f269cac55f7df989edf1c7dc2c82040fee75ddce602a015ac897bb9b05068"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L61-61"
    excerpt_sha256: "sha256:94e9dea507f50aff153aeef705a9cd9b10532fdea612161b75a2d515fd52a3e5"
---
# Relations
- Producer context: [analytics](/contexts/analytics)
- Consumer context: [identity](/contexts/identity)
