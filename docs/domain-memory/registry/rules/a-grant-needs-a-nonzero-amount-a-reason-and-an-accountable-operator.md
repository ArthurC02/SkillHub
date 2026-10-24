---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L359-374"
    excerpt_sha256: "sha256:8806b258fe65c4f5ad9e7a1f4d561bab7f56b6aa779d8b9c19e5c204e36d62ad"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L364-378"
    excerpt_sha256: "sha256:f574783edfd264c90cf378c0716149fe2bdf7463f8e041ffef638be6eb6a88f6"
---
An operator entry is refused when its credits are zero, its trimmed reason is empty, or its user, operator or idempotency key is missing; its kind must be grant, topup or adjustment.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A 50-credit grant whose reason is only spaces is refused.
