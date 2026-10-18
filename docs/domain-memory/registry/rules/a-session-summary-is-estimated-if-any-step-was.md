---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T08:44:17.756466Z"
sources:
  - resource: "apps/platform/internal/creator/credit/summary.go#L27-37"
    excerpt_sha256: "sha256:ef999a58374c928fec77b47e5d737261b2722266dab4c1533247b0973c83cfbf"
  - resource: "apps/platform/internal/creator/credit/summary_test.go#L10-38"
    excerpt_sha256: "sha256:ebb82513356d756f044d7b54821192392a33f7a44b74189611ee20628d6adf72"
---
A session cost summary adds every step, belongs to the user of the last step, and is estimated when any step's cost was estimated.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Steps priced by gateway, estimate and gateway give an estimated summary of three steps.
