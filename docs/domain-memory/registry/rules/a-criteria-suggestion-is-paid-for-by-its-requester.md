---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggest_credit_integration_test.go#L55-96"
    excerpt_sha256: "sha256:354e7871e1e63baa7eb16aa4708f2668691f5db5541b39b7a3a27427e463af06"
  - resource: "apps/platform/internal/trial/design/cost.go#L22-38"
    excerpt_sha256: "sha256:f1877a709e43f98fd927ff215c6c577ac8936f2de533870f1a7589689e79b748"
  - resource: "apps/platform/internal/trial/design/suggest.go#L67-70"
    excerpt_sha256: "sha256:33c1cc1a152d3e4539e0104f487240c34cffae4aed44e20a0b1237ee47418b49"
  - resource: "apps/platform/internal/trial/design/suggest.go#L106-108"
    excerpt_sha256: "sha256:8afa163d805819368d07518a001326a34747b86961d5058d7e6ec2bfc7b64ab5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/preflight_integration_test.go#L696-732"
    excerpt_sha256: "sha256:592c6f641a0c6ee7c64c6b0c60411bcf2f71502e48d4088762f23d23512826d8"
  - resource: "apps/platform/internal/trial/design/suggest.go#L72-74"
    excerpt_sha256: "sha256:59952126e3d2651a7268888dc36abf67669d23566a5a623df2a050efeb443850"
---
Suggesting acceptance criteria is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the test case already holds the most criteria it may or when the owner's credit balance is below the start threshold for suggest_criteria, and debited by the usage the gateway reported once the model answers; it never proposes more criteria than there is room for.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With no suggest_criteria statistics the threshold is the 70-credit fallback: a balance of 69 is refused with no model call and nothing charged; a balance of 70 is allowed and a $0.01 answer debits 13 credits at the default markup.
- With 19 of 20 criteria a suggestion answers with one proposal out of three; with 20 it answers 413 and the model is not called.
