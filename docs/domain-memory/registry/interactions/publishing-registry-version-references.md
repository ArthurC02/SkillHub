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
    excerpt_sha256: "sha256:7037d57ed2d2592a8edb3a1773b456d4d78a6096b9890836d7b36df52aba69a4"
---
# Relations
- Producer context: [publishing](/contexts/publishing)
- Consumer context: [registry](/contexts/registry)
