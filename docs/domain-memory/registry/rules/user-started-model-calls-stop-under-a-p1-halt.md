---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/trial/design/cost.go#L22-37"
    excerpt_sha256: "sha256:6df644766cd397f4d57227d46082b1eb508bcbc096b2a7fa0ba43fb6a635b63b"
  - resource: "apps/platform/internal/creator/creation/job.go#L453-455"
    excerpt_sha256: "sha256:3f7684b057cdb1f15bb4f1130f90075c7504a2c745b24184a0ab12cebdd21266"
  - resource: "apps/platform/internal/creator/creation/job.go#L622-627"
    excerpt_sha256: "sha256:40352e3aaee09c5e33a426829b41a169b70e2bb148976f1edf54f28a2482c3e9"
  - resource: "apps/platform/internal/trial/execution/halt.go#L32-46"
    excerpt_sha256: "sha256:d230881335bad2a2fccc01812cfdaf34794d0fd3c247b3ab0973dc5ef85d14d3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/p1_halt_model_calls_integration_test.go#L18-81"
    excerpt_sha256: "sha256:d8e8d8a5a0c369b973d2acacea3d5df6440e1e5f55fe9cae4bdc2618640a43fd"
  - resource: "apps/platform/internal/skill/admission/generate.go#L254-256"
    excerpt_sha256: "sha256:a210cb9d749a2bb04a447d71aacfb15c1e6f37e3946db16c419a030afdd5be2b"
  - resource: "apps/platform/internal/skill/admission/enrich.go#L117-119"
    excerpt_sha256: "sha256:b2e935bc0ef591576f4c3882d619298a1f09b40ebee0cd46eecc286a86e1ee35"
  - resource: "apps/platform/internal/skill/admission/service.go#L555-557"
    excerpt_sha256: "sha256:b85be66aa7f8c261615dbffbb702ba8d542ff49022a70d7228372bf817b0ad13"
  - resource: "apps/platform/internal/skill/admission/cost_test.go#L185-215"
    excerpt_sha256: "sha256:5c9abfbf675a78c854e387133a55471ecb709d64727d5833963dcca4cc66bcc1"
---
While a P1 incident halts dispatch, generating a skill, a creation step, a criteria suggestion and the enrichment of an imported version are refused before the model is called and nothing is charged; a creation step that was refused is not counted and the session waits for the person to send again, and an import still completes with its enrichment left pending for the backfill, which picks nothing up until the halt is lifted. An unreadable halt state counts as halted.

# Relations
- Contexts: [creation](/contexts/creation)
- Contexts: [ingest](/contexts/ingest)
- Contexts: [testlab](/contexts/testlab)

# Examples
- Under a P1 halt a criteria suggestion answers 503 naming the halt, generation returns ErrModelCallsHalted, and the model stub is never called.
- An import under a P1 halt is stored with its enrichment pending and the model stub never called; the backfill run under the halt enriches nothing and the version stays pending.
