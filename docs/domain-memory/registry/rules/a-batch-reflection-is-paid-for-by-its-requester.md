---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L58-73"
    excerpt_sha256: "sha256:a48a8a4003f7a809025cc72a585650247820686c77ae6f1f05b8d2434b258816"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L218-254"
    excerpt_sha256: "sha256:c61543833faaeb83b7fda4ff296d901954966fe410fc615c69394e144f7b0a9a"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L331-347"
    excerpt_sha256: "sha256:e9f743c971e08aac0206b46204cdcf8e8215aa5689e83a5e3c65ad7dd82b031d"
---
An improvement round over an evaluation batch is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the owner's credit balance is below the start threshold for batch_reflection or model calls are halted, and debited by the usage the gateway reported once the model answers.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- With no batch_reflection statistics a balance of 69 credits is refused with 422, the model is not called and nothing is charged.
