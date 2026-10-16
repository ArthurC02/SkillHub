---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T17:57:56.233858Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/service.go#L326-336"
    excerpt_sha256: "sha256:b1e8edde038cda98195cb6498b6aeb6fdf655c1b6a27bceb053934db679cba77"
  - resource: "apps/platform/internal/creator/workspace/account.go#L41-57"
    excerpt_sha256: "sha256:8ff1053d6d1caa19b5c0fe2476c207a4fb6a2d5b54a35f1561008d99f11d8808"
  - resource: "apps/platform/internal/creator/workspace/account_test.go#L34-63"
    excerpt_sha256: "sha256:c10ecde98da22afc1a1f47357f055a4302fe11bcce8c696fb637999ab8a1525c"
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
