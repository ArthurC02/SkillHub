---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/creation_wiring.go#L90-90"
    excerpt_sha256: "sha256:9d58681e1be67a44e6762288aff538a0002b9b7d0f2285bf5f8093d3a50d579d"
---
# Relations
- Producer context: [registry](/contexts/registry)
- Consumer context: [creation](/contexts/creation)
