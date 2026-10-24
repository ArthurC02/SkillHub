---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/postgres.go#L97-117"
    excerpt_sha256: "sha256:74cc54b7abf930eaca81a2c4dbf63aaad8da82b69882be963c7539bf5f8981de"
  - resource: "apps/platform/internal/creator/credit/service.go#L154-169"
    excerpt_sha256: "sha256:ed46f19c08d56093bebfa9abae115f13a530be12d901ece13272e4381c79d214"
  - resource: "apps/platform/internal/creator/credit/postgres_integration_test.go#L151-200"
    excerpt_sha256: "sha256:865b49bbf664dd06a0889fd18915bc7e6b3c0e30f52ba9d2e7131c1bb47c42e8"
---
Replaying a charge under the same idempotency key reports it as existing and leaves one cost event, one ledger entry and one debit.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Charging the same creation-step key twice debits 13 credits once.
