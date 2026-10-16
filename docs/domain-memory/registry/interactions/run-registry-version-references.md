---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L207-207"
    excerpt_sha256: "sha256:627a19bc1637f2d2ee841e00bb6f806a216432551bed4edd908464352da53e50"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [registry](/contexts/registry)
