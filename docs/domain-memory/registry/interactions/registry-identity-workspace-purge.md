---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L221-221"
    excerpt_sha256: "sha256:f4912f71c9d9e65eb60e93bfd40c742fd2e7f362d058b3e80449bea40f8f08bc"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L60-60"
    excerpt_sha256: "sha256:3e131ca4d5a1a5c3cd4ccded7be231c677608ec7e3023f63f1197722d1801e96"
---
# Relations
- Producer context: [registry](/contexts/registry)
- Consumer context: [identity](/contexts/identity)
