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
    excerpt_sha256: "sha256:9294d276b1997a4340eca9cbaffa3d1b553e0570fa05c5f5b05ac1dacedc5b5c"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [registry](/contexts/registry)
