---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L221-221"
    excerpt_sha256: "sha256:f4912f71c9d9e65eb60e93bfd40c742fd2e7f362d058b3e80449bea40f8f08bc"
---
# Relations
- Producer context: [registry](/contexts/registry)
- Consumer context: [identity](/contexts/identity)
