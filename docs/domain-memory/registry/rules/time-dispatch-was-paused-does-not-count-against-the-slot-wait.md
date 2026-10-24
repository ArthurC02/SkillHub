---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/execution/halt/paused.go#L22-41"
    excerpt_sha256: "sha256:6a494406a7a0e101d3edc8c775d18d6b95a3698ac29cde59a9c6151ad9937511"
  - resource: "apps/platform/internal/trial/execution/job.go#L952-958"
    excerpt_sha256: "sha256:7124a1a3ceb89a3de9554dc4546c222d03838b09c22e36b26c2d2a59f6f30202"
  - resource: "apps/platform/internal/trial/execution/job.go#L969-978"
    excerpt_sha256: "sha256:0d77c9e792b2cb23b7b852fdcd35592800d6833406652a22d80df5be13307ed7"
  - resource: "apps/platform/internal/trial/execution/halt/paused_test.go#L28-54"
    excerpt_sha256: "sha256:d5046eb497c504ce795484fd8a57591cf66fde02fcccb5e71475fb3b90442a5c"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/p1_halt_model_calls_integration_test.go#L83-105"
    excerpt_sha256: "sha256:766f13f28c07d0625db0bd84a1a83800323f6dd9cbf92211048bfa76c859cc70"
---
A run waiting for a sandbox slot times out only after the slot-wait limit plus every stretch during which dispatch was paused for every provider that could take it; if the halt history cannot be read the run keeps waiting.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run queued forty minutes ago, fifteen of them under a pool halt, is still queued; a pool halt declared before the run queued and still held stops its clock entirely.
