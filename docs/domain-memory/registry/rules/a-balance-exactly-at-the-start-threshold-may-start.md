---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T08:44:17.756466Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L199-215"
    excerpt_sha256: "sha256:89d86c46f2967ecea8a38db26a6adf304699937c1e77f8bf3dd77bc9e0a8669a"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L292-309"
    excerpt_sha256: "sha256:7b99d21ab7b8cfcd893debf6475ebd06e3d934140e033931c93368ce19f6f331"
---
A start is allowed when the balance is at or above the start threshold; one credit below is refused.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- With a threshold of 70, a balance of 70 may start and 69 may not.
