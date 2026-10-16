---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L215-215"
    excerpt_sha256: "sha256:d46152cb58f3e63ae83b091a659644a71bb98edaaa9e641779d94f1cde1b0364"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [identity](/contexts/identity)
