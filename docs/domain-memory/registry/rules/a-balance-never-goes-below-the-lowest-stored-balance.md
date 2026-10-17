---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:41:06.695372Z"
sources:
  - resource: "apps/platform/internal/creator/credit/postgres.go#L153-159"
    excerpt_sha256: "sha256:9f9bd680d69cddd3e5b78dd7effb5fdb6978c4c6d8ce28253e1eba19f2f6e4b0"
  - resource: "apps/platform/internal/creator/credit/postgres.go#L182-185"
    excerpt_sha256: "sha256:b406ed390b8f367075c22ddc07b1bf079784717034c5c4bdca68da73b612588f"
  - resource: "apps/platform/internal/creator/credit/postgres_integration_test.go#L59-100"
    excerpt_sha256: "sha256:f58c03d11586a4afd8d7802a68dc97df1a6cc53ab3dbd3daca514faa79b60909"
---
An entry that would take a balance below MinBalanceCredits is refused with ErrBalanceOutOfRange and nothing is applied; landing exactly on the minimum is applied.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- An adjustment of MinBalanceCredits-1 on a fresh account is refused and the balance stays 0.
