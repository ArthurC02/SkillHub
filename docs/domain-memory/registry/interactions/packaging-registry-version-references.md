---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L209-209"
    excerpt_sha256: "sha256:e15e580dafcd244d704d134b27136d2bcb90fbb118cd7fb942afeddc4b1fda80"
---
# Relations
- Producer context: [packaging](/contexts/packaging)
- Consumer context: [registry](/contexts/registry)
