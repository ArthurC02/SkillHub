---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L223-223"
    excerpt_sha256: "sha256:0070fc188562407256874292cc699b576ce5823cee930fc8561eb4f2194e2609"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L66-66"
    excerpt_sha256: "sha256:3dce59b19afa245cbf9233679a473a6f12e977232e48b7a996bea39f17c1418d"
---
# Relations
- Producer context: [eval](/contexts/eval)
- Consumer context: [identity](/contexts/identity)
