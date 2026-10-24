---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L303-305"
    excerpt_sha256: "sha256:022428359262ed9fc6a4357ca45c92c8c6c3f5c388a39581707769114e60751a"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L103-112"
    excerpt_sha256: "sha256:df295e6db55c705d30911735f422922e6b311446fd060b5eb79b3563679621e6"
---
While a creation is queued for or working with the model, only cancel and stop-step are accepted; other commands conflict.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Answering a question while the model works conflicts.
