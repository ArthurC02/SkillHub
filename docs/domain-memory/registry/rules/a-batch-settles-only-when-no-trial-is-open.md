---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L48-59"
    excerpt_sha256: "sha256:54b626746fdb5d35b1a8a9e257a0876f89c464f7a4f335b38ce278d206b7d874"
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L65-78"
    excerpt_sha256: "sha256:db70f9be6da0b8a45b9feecb10b002702c064a61007b5f7c859627adac4d5ef9"
---
A batch is marked settled only once none of its trials is still running or owed an answer.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch whose next trial is still running after a simulated answer stays unsettled.
