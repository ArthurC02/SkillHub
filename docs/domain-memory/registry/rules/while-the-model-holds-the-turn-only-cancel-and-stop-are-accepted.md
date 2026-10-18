---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L300-302"
    excerpt_sha256: "sha256:022428359262ed9fc6a4357ca45c92c8c6c3f5c388a39581707769114e60751a"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L101-110"
    excerpt_sha256: "sha256:df295e6db55c705d30911735f422922e6b311446fd060b5eb79b3563679621e6"
---
While a creation is queued for or working with the model, only cancel and stop-step are accepted; other commands conflict.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Answering a question while the model works conflicts.
