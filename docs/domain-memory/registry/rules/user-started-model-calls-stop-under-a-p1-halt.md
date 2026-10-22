---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/cost.go#L22-37"
    excerpt_sha256: "sha256:6df644766cd397f4d57227d46082b1eb508bcbc096b2a7fa0ba43fb6a635b63b"
  - resource: "apps/platform/internal/skill/admission/generate.go#L253-255"
    excerpt_sha256: "sha256:3f7684b057cdb1f15bb4f1130f90075c7504a2c745b24184a0ab12cebdd21266"
  - resource: "apps/platform/internal/creator/creation/job.go#L445-447"
    excerpt_sha256: "sha256:3f7684b057cdb1f15bb4f1130f90075c7504a2c745b24184a0ab12cebdd21266"
  - resource: "apps/platform/internal/creator/creation/job.go#L614-619"
    excerpt_sha256: "sha256:40352e3aaee09c5e33a426829b41a169b70e2bb148976f1edf54f28a2482c3e9"
  - resource: "apps/platform/internal/trial/execution/halt.go#L32-46"
    excerpt_sha256: "sha256:d230881335bad2a2fccc01812cfdaf34794d0fd3c247b3ab0973dc5ef85d14d3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/p1_halt_model_calls_integration_test.go#L18-81"
    excerpt_sha256: "sha256:d8e8d8a5a0c369b973d2acacea3d5df6440e1e5f55fe9cae4bdc2618640a43fd"
---
While a P1 incident halts dispatch, generating a skill, a creation step and a criteria suggestion are refused before the model is called and nothing is charged; a creation step that was refused is not counted and the session waits for the person to send again. An unreadable halt state counts as halted.

# Relations
- Contexts: [creation](/contexts/creation)
- Contexts: [ingest](/contexts/ingest)
- Contexts: [testlab](/contexts/testlab)

# Examples
- Under a P1 halt a criteria suggestion answers 503 naming the halt, generation returns ErrModelCallsHalted, and the model stub is never called.
