---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/suggest.go#L64-66"
    excerpt_sha256: "sha256:59952126e3d2651a7268888dc36abf67669d23566a5a623df2a050efeb443850"
  - resource: "apps/platform/internal/trial/design/cost.go#L18-30"
    excerpt_sha256: "sha256:8f98309f6ddd0b5dab1b9c1225960799ad3ff203121106f80aaea6972031a5ee"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggest_credit_integration_test.go#L55-96"
    excerpt_sha256: "sha256:354e7871e1e63baa7eb16aa4708f2668691f5db5541b39b7a3a27427e463af06"
---
Suggesting acceptance criteria is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the owner's credit balance is below the start threshold for suggest_criteria, and debited by the usage the gateway reported once the model answers.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With no suggest_criteria statistics the threshold is the 70-credit fallback: a balance of 69 is refused with no model call and nothing charged; a balance of 70 is allowed and a $0.01 answer debits 13 credits at the default markup.
