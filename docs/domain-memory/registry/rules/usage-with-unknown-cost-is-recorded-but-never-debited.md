---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T12:28:17.483821Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L95-112"
    excerpt_sha256: "sha256:d964d35b41fa167f4e9f60969912b2ed11a5c5a652d1554b5f276a660e1b7954"
  - resource: "apps/platform/internal/creator/credit/service.go#L155-162"
    excerpt_sha256: "sha256:8a0e5c729dc2ac07aff5309d092f73cda6edf0e33985fa8c62a647b4c241942e"
  - resource: "apps/platform/internal/creator/credit/service.go#L537-544"
    excerpt_sha256: "sha256:abc1c8c606a978a6550b64ece4af92f170a0971628cc1d15b504494b1efb3550"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L144-169"
    excerpt_sha256: "sha256:6152f0ffdcdf942ab68bfb6437f197ac9a063f357feaa2f4edc112141bebbbb2"
---
A charge whose actual cost is unknown is recorded as an estimated cost event and debits no credits; the reserved ceiling is never billed as the cost.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- An unpriced call leaves the balance unchanged and records an estimated cost event.
