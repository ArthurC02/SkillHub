---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L210-210"
    excerpt_sha256: "sha256:e26bc149ef018334dc26aa8258b4d7553af266f31a16eaf0fe14f027f0f14e5f"
---
# Relations
- Producer context: [publishing](/contexts/publishing)
- Consumer context: [registry](/contexts/registry)
