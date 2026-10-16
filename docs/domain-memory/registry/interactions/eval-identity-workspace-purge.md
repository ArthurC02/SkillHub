---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T03:13:57.144861Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L219-219"
    excerpt_sha256: "sha256:875e512562f8552c6e2fe2e0c217a9d4a27edbebffd97420dd9e08bb452b1473"
---
# Relations
- Producer context: [eval](/contexts/eval)
- Consumer context: [identity](/contexts/identity)
