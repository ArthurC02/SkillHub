---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L184-190"
    excerpt_sha256: "sha256:0cd436a386eb11960d8654d65c811c6239b6a105d6b52beb7e7828450dd36801"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L238-257"
    excerpt_sha256: "sha256:0c034df5c58b06a3e32795dad570d3292e6d706888e1945cdfa485a3c00670b1"
---
A paid step may proceed only when the balance minus the credits billed for its reservation stays at or above the configured debt floor.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- With a floor of -50 and 13 credits reserved, a balance of -37 may continue and -38 may not.
