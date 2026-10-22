---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L211-211"
    excerpt_sha256: "sha256:8a19f2f76fc08b0b256a21e00b3f06b218e56ca69e9283578561465901a80053"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [registry](/contexts/registry)
