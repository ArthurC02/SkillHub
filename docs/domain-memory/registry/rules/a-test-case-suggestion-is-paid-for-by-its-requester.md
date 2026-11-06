---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T08:43:09.009606Z"
sources:
  - resource: "apps/platform/internal/trial/design/suggest_test_cases.go#L72-125"
    excerpt_sha256: "sha256:6c9137515b3f5f0218a05d3597eab87d940367d0094f9ba1f82b7dab728413df"
  - resource: "apps/platform/internal/trial/design/suggest_test_cases.go#L137-155"
    excerpt_sha256: "sha256:8c7a821c2f3fb65025fcd242f1a8e64df4aab0eef974c4592726d1a701765593"
  - resource: "apps/platform/internal/trial/design/suggest_test_cases.go#L127-135"
    excerpt_sha256: "sha256:438b7ffc12b3f96469f240a598e4c0856ead6be35b153ef20677dd1d9f18d7d2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggest_test_cases_integration_test.go#L71-89"
    excerpt_sha256: "sha256:2ce708d45e29c3e7330981dfc875532ee7b8bb2860d648259631ad59813c0d38"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggest_test_cases_integration_test.go#L91-131"
    excerpt_sha256: "sha256:d02140904bf91dac955dff6b425e36a8e57823173016f0bf328bda760c200f63"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggest_test_cases_integration_test.go#L169-194"
    excerpt_sha256: "sha256:b1753032bc6eecc4191eedc08f89c7b77e03e326dc80c283d9782ffbad399d14"
---
Suggesting test cases for a skill is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the owner's credit balance is below the start threshold for suggest_test_cases or model calls are halted, and debited by the usage the gateway reported once the model answers; it stores nothing, and a proposal whose name the skill already uses is dropped.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With no suggest_test_cases statistics the threshold is the 70-credit fallback: a balance of 69 is refused with no model call and nothing charged; a balance of 70 is allowed and a $0.01 answer debits 13 credits and records one suggest_test_cases cost event.
- A proposal named like the skill's existing test case is dropped, and the skill still has only the test cases it had before the call.
