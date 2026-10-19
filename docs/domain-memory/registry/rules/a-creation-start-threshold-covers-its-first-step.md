---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T16:29:33.216329Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L238-238"
    excerpt_sha256: "sha256:516ed3ed70c60ff26881d99b0a1440747f5e45c11ed291294789bada6ff7873f"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L278-315"
    excerpt_sha256: "sha256:9c4590fb3639d39fae661cd9418af8050e87fcdd5979b585dacc2d11f5ac8653"
---
A creation session may start only when the balance also covers its first step: the start threshold is at least the first step's reserve in credits plus the debt floor, whatever the statistics say.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- With a US$0.10 step reserve, markup 1.3 and floor -50, a balance of 79 cannot start and 80 can.
