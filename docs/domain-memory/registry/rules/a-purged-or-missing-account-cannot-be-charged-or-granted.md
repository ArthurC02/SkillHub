---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L46-58"
    excerpt_sha256: "sha256:2578d8f9055751a6201744f7180225f09c45f79dc164de0819caf77afb89e26c"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L479-497"
    excerpt_sha256: "sha256:a419941f99fb7fceab65fbe651440614874553d83ac80ff449550cdab4460b85"
---
When account facts report the user missing or purged, a grant and a charge are refused with ErrAccountGone and nothing is recorded.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A charge to a purged account leaves no cost event and no debit.
