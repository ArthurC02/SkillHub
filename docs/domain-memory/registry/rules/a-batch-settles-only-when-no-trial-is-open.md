---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T09:17:06.348002Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L48-59"
    excerpt_sha256: "sha256:54b626746fdb5d35b1a8a9e257a0876f89c464f7a4f335b38ce278d206b7d874"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_simulated_user_integration_test.go#L79-113"
    excerpt_sha256: "sha256:6c5df44dca15ccf4b7f9877ffc4b88b50436d234cf06b4828dda18a6ebc23a5f"
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L61-85"
    excerpt_sha256: "sha256:409230ac2c9b3e81ac0b65cc181c5f328bf8e355984b7ebf242a0a12658de6ec"
---
A batch is marked settled only once none of its trials is still running or owed an answer; a paused batch is neither answered nor settled until it is resumed.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch whose next trial is still running after a simulated answer stays unsettled.
