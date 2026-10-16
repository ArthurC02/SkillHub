---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L216-216"
    excerpt_sha256: "sha256:d46152cb58f3e63ae83b091a659644a71bb98edaaa9e641779d94f1cde1b0364"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L51-51"
    excerpt_sha256: "sha256:03fb2d7b1d5ed909f511a3f6d350d580f7ad458bb32057023a97ca5e1b91c657"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [identity](/contexts/identity)
