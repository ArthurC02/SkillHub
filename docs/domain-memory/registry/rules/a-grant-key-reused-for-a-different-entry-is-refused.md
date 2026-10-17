---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/postgres.go#L166-178"
    excerpt_sha256: "sha256:0753543290451865f1bb118b6ea518dea7c72975ec60c5d618135db3aaa8a6be"
  - resource: "apps/platform/internal/creator/credit/postgres_integration_test.go#L102-149"
    excerpt_sha256: "sha256:201b6b104284653458199a67b657139f04029123c1d8de6df7810a1ddd8f956b"
---
An idempotency key already used for an operator entry is refused with ErrGrantKeyReused when it is presented with another kind or another amount, and the balance is unchanged.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Key k granted 50; k presented again for 60 or as a topup is refused and the balance stays 50.
