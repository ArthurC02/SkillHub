---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L65-74"
    excerpt_sha256: "sha256:285a06116bfd5191ac103086cbd1ae7caffa575abf1fdd80bca4b77cbb70d546"
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L97-115"
    excerpt_sha256: "sha256:dd2b04e5c5e4a79fdeb7fc402ba0c70b0c71e721507e877b3048980f1bfce84e"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_simulated_user_integration_test.go#L213-258"
    excerpt_sha256: "sha256:1ab82cc01935301e00f061ed428abcafc632746a3ffc9d871440f32d97a823b5"
---
When Run refuses a simulated person's answer because the owner's credit or run quota ran out, the batch pauses with that reason instead of closing the trial; a paused batch answers nothing, and resuming it gives the answers still owed.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- An answer refused for credit_balance leaves the batch paused and the trial unanswered; topping up without resuming still answers nothing; resuming answers it and the batch is launched again.
