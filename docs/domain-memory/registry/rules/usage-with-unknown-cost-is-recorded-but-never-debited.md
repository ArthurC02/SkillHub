---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:41:06.695372Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L115-119"
    excerpt_sha256: "sha256:c53e8e188f9f5771c4cdcc0236e21066793714ed1839b8394d4885b9d5f4d06b"
  - resource: "apps/platform/internal/creator/credit/service.go#L144-151"
    excerpt_sha256: "sha256:8a0e5c729dc2ac07aff5309d092f73cda6edf0e33985fa8c62a647b4c241942e"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L143-168"
    excerpt_sha256: "sha256:6152f0ffdcdf942ab68bfb6437f197ac9a063f357feaa2f4edc112141bebbbb2"
---
A charge whose actual cost is unknown is recorded as an estimated cost event and debits no credits; the reserved ceiling is never billed as the cost.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- An unpriced call leaves the balance unchanged and records an estimated cost event.
