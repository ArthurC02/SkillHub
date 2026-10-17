---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:16:09.817174Z"
sources:
  - resource: "apps/platform/internal/creator/creation/state.go#L35-107"
    excerpt_sha256: "sha256:9adca72d292652876bb34bcfb9e8aaeccd270cd04c3295430167283b57dd9327"
  - resource: "apps/platform/internal/creator/creation/state_test.go#L95-107"
    excerpt_sha256: "sha256:720926e694e872cd0b6b2c6985ad991871ecb23539c1bd9870b63d1132e7d5f5"
---
A creation session reaches draft_ready only through the model's step; no other transition may produce a draft.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- waiting_input to draft_ready is an illegal transition.
