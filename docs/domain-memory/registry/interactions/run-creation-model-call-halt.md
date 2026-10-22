---
type: Interaction
status: stable
consistency: synchronous
delivery: composition-root hook
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T16:18:22.504359Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L654-661"
    excerpt_sha256: "sha256:a3328498e75275f977844508c68f206866494f6306b7df4b4641313603bebb32"
  - resource: "apps/platform/internal/entrypoint/worker/worker.go#L131-134"
    excerpt_sha256: "sha256:1d6e9d8aec939b04af79e97b2f941065b6a3dab4787c6fc48793c1462b5dc9c9"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer context: [creation](/contexts/creation)
