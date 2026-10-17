---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/state.go#L35-107"
    excerpt_sha256: "sha256:9adca72d292652876bb34bcfb9e8aaeccd270cd04c3295430167283b57dd9327"
---
A creation session reaches draft_ready only through the model's step; no other transition may produce a draft.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- waiting_input to draft_ready is an illegal transition.
