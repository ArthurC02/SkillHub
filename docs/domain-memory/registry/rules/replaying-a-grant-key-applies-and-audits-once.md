---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L379-388"
    excerpt_sha256: "sha256:ebf6d5024dbf03c729ef4fd7e1ecb3eec09060465a6f16c6f0b83a8546b617f6"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L437-458"
    excerpt_sha256: "sha256:ef5d04abbe1616793a579ae5cc1cd81c9ecd8266a93a826aef11d4c37e55a006"
---
Re-submitting an operator entry under the same idempotency key returns the current balance without applying it or writing a second audit entry.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Two identical 50-credit grants under one key leave the balance at 50 with one audit entry.
