---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L208-208"
    excerpt_sha256: "sha256:b4c5215f57c9cca83caa1e475d168f9802150dd8ca33e756b6914a35da7b5e73"
---
# Relations
- Producer context: [packaging](/contexts/packaging)
- Consumer context: [registry](/contexts/registry)
