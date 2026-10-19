---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T18:30:19.836130Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/service.go#L329-339"
    excerpt_sha256: "sha256:0520b7e31b141aaa3546d12c08c04a33e92b5fbd10365eea013591ad41877248"
  - resource: "apps/platform/internal/creator/workspace/account.go#L42-69"
    excerpt_sha256: "sha256:f299a7a5956ac36a9374f06b675c2cf5b0a10b91aa65a2ff4c2b9aa2f8ca95f8"
  - resource: "apps/platform/internal/creator/workspace/account_test.go#L34-71"
    excerpt_sha256: "sha256:e877185dc0ac36b0f6dd870720f86296ffcec89f918b1e66c652b157213cf838"
  - resource: "contracts/openapi/public.yaml#L195-208"
    excerpt_sha256: "sha256:cbdf684053ba1daf32836bbeb0309454cad0c581e5541945e39a3992744ecf53"
---
Requesting account deletion starts a 30-day grace period during which the Account stays fully usable; asking again keeps the original request time instead of extending the wait, and cancelling clears the request and is harmless when nothing is pending.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A second DELETE /me keeps the first deletion_requested_at and the same purge_after.
- POST /me/deletion/cancel with nothing pending answers 200 with deletion_requested_at null.
- A fresh request after a cancel starts a new clock with no purge attempt on record.
