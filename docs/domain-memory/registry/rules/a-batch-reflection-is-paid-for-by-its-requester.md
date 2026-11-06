---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L58-73"
    excerpt_sha256: "sha256:a48a8a4003f7a809025cc72a585650247820686c77ae6f1f05b8d2434b258816"
---
An improvement round over an evaluation batch is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the owner's credit balance is below the start threshold for batch_reflection or model calls are halted, and debited by the usage the gateway reported once the model answers.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- With no batch_reflection statistics a balance of 69 credits is refused with 422, the model is not called and nothing is charged.
